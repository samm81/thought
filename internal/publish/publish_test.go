package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samm81/thought/internal/archive"
	"github.com/samm81/thought/internal/metadata"
	"github.com/samm81/thought/internal/xpost"
)

const (
	responsePublished = "published"
	errorTransport    = "transport"
)

type fakeClient struct {
	responses []xpost.Response
	requests  []xpost.Request
}

type errorClient struct {
	err     error
	request int
}

func (c *errorClient) Publish(_ context.Context, _ xpost.Request) (xpost.Response, error) {
	c.request++
	return xpost.Response{}, c.err
}

func (c *fakeClient) Publish(_ context.Context, request xpost.Request) (xpost.Response, error) {
	c.requests = append(c.requests, request)
	response := c.responses[0]
	c.responses = c.responses[1:]

	return response, nil
}

func TestNeedsPublication(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first")

	needs, err := NeedsPublication(thought, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !needs {
		t.Fatal("NeedsPublication() = false, want true for pending thought")
	}

	publication := metadata.New([]string{"01"})
	for _, target := range metadata.Targets() {
		record := publication.Get("01", target)
		if err := record.MarkPublishing(time.Now()); err != nil {
			t.Fatal(err)
		}

		if err := record.MarkPublished(time.Now(), metadata.Reference{ID: target + "-1"}, ""); err != nil {
			t.Fatal(err)
		}

		if err := publication.Set("01", target, record); err != nil {
			t.Fatal(err)
		}
	}

	if err := metadata.Save(thought.MetadataPath(), publication); err != nil {
		t.Fatal(err)
	}

	needs, err = NeedsPublication(thought, nil)
	if err != nil {
		t.Fatal(err)
	}

	if needs {
		t.Fatal("NeedsPublication() = true, want false for published thought")
	}
}

func TestPublishThreadsAndRecordsReferences(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first", "02.md", "second")
	client := &fakeClient{responses: []xpost.Response{
		{Status: responsePublished, RemoteID: "at://first", RemoteCID: "cid-first"},
		{Status: responsePublished, RemoteID: "at://second", RemoteCID: "cid-second"},
	}}
	publisher := New(client)

	publisher.now = func() time.Time { return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC) }
	if err := publisher.Publish(context.Background(), thought, []string{metadata.TargetBluesky}, nil); err != nil {
		t.Fatal(err)
	}

	if len(client.requests) != 2 || client.requests[1].ReplyTo == nil || client.requests[1].ReplyTo.ID != "at://first" {
		t.Fatalf("requests = %#v", client.requests)
	}

	publication, err := metadata.Load(thought.MetadataPath(), []string{"01", "02"})
	if err != nil {
		t.Fatal(err)
	}

	if publication.Get("02", metadata.TargetBluesky).Status != metadata.StatePublished {
		t.Fatalf("second state = %q", publication.Get("02", metadata.TargetBluesky).Status)
	}

	if filepath.Dir(thought.MetadataPath()) != thought.Path() {
		t.Fatalf("metadata path is outside thought")
	}
}

func TestPublishStopsAfterFailure(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first", "02.md", "second", "03.md", "third")
	client := &fakeClient{responses: []xpost.Response{
		{Status: responsePublished, RemoteID: "at://first", RemoteCID: "cid-first"},
		{Status: "failed", ErrorKind: errorTransport, Error: "timeout"},
	}}

	publisher := New(client)
	if err := publisher.Publish(context.Background(), thought, []string{metadata.TargetBluesky}, nil); err == nil {
		t.Fatal("Publish() error = nil, want failure")
	}

	if len(client.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(client.requests))
	}

	publication, err := metadata.Load(thought.MetadataPath(), []string{"01", "02", "03"})
	if err != nil {
		t.Fatal(err)
	}

	if got := publication.Get("02", metadata.TargetBluesky).Status; got != metadata.StateFailed {
		t.Fatalf("failed state = %q", got)
	}

	if got := publication.Get("03", metadata.TargetBluesky).Status; got != metadata.StatePending {
		t.Fatalf("blocked state = %q", got)
	}
}

func TestPublishKeepsTargetsIndependent(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first")
	client := &fakeClient{responses: []xpost.Response{
		{Status: "failed", ErrorKind: errorTransport, Error: "network"},
		{Status: responsePublished, RemoteID: "x-1"},
	}}

	publisher := New(client)
	if err := publisher.Publish(context.Background(), thought, nil, nil); err == nil {
		t.Fatal("Publish() error = nil, want one target failure")
	}

	publication, err := metadata.Load(thought.MetadataPath(), []string{"01"})
	if err != nil {
		t.Fatal(err)
	}

	if got := publication.Get("01", metadata.TargetBluesky).Status; got != metadata.StateFailed {
		t.Fatalf("Bluesky state = %q", got)
	}

	if got := publication.Get("01", metadata.TargetX).Status; got != metadata.StatePublished {
		t.Fatalf("X state = %q", got)
	}
}

func TestPublishRetriesFailedPost(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first")
	publication := metadata.New([]string{"01"})

	record := publication.Get("01", metadata.TargetBluesky)
	if err := record.MarkPublishing(time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := record.MarkFailed(errorTransport, "timeout"); err != nil {
		t.Fatal(err)
	}

	if err := publication.Set("01", metadata.TargetBluesky, record); err != nil {
		t.Fatal(err)
	}

	if err := metadata.Save(thought.MetadataPath(), publication); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{responses: []xpost.Response{{Status: responsePublished, RemoteID: "post-1"}}}
	if err := New(client).Publish(context.Background(), thought, []string{metadata.TargetBluesky}, nil); err != nil {
		t.Fatal(err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(client.requests))
	}

	loaded, err := metadata.Load(thought.MetadataPath(), []string{"01"})
	if err != nil {
		t.Fatal(err)
	}

	if got := loaded.Get("01", metadata.TargetBluesky).Status; got != metadata.StatePublished {
		t.Fatalf("status = %q, want published", got)
	}
}

func TestPublishMarksCanceledPostFailed(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first")

	client := &errorClient{err: context.Canceled}
	if err := New(client).Publish(context.Background(), thought, []string{metadata.TargetBluesky}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Publish() error = %v, want context canceled", err)
	}

	if client.request != 1 {
		t.Fatalf("request count = %d, want 1", client.request)
	}

	loaded, err := metadata.Load(thought.MetadataPath(), []string{"01"})
	if err != nil {
		t.Fatal(err)
	}

	record := loaded.Get("01", metadata.TargetBluesky)
	if record.Status != metadata.StateFailed {
		t.Fatalf("status = %q, want failed", record.Status)
	}

	if record.ErrorKind != errorTransport || record.Error != context.Canceled.Error() {
		t.Fatalf("failure = %#v, want transport cancellation", record)
	}
}

func TestPublishRejectsAndStopsThread(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first", "02.md", "second")

	client := &fakeClient{responses: []xpost.Response{{Status: "rejected", Error: "too long"}}}
	if err := New(client).Publish(context.Background(), thought, []string{metadata.TargetBluesky}, nil); err == nil {
		t.Fatal("Publish() error = nil, want rejection")
	}

	if len(client.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(client.requests))
	}

	loaded, err := metadata.Load(thought.MetadataPath(), []string{"01", "02"})
	if err != nil {
		t.Fatal(err)
	}

	if got := loaded.Get("01", metadata.TargetBluesky).Status; got != metadata.StateRejected {
		t.Fatalf("first status = %q, want rejected", got)
	}

	if got := loaded.Get("02", metadata.TargetBluesky).Status; got != metadata.StatePending {
		t.Fatalf("second status = %q, want pending", got)
	}

	if !strings.Contains(loaded.Get("01", metadata.TargetBluesky).Error, "too long") {
		t.Fatalf("rejection error = %q", loaded.Get("01", metadata.TargetBluesky).Error)
	}
}

func TestPublishUsesFallbackResponseError(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first")

	client := &fakeClient{responses: []xpost.Response{{Status: "failed"}}}
	if err := New(client).Publish(context.Background(), thought, []string{metadata.TargetBluesky}, nil); err == nil {
		t.Fatal("Publish() error = nil, want failure")
	}

	loaded, err := metadata.Load(thought.MetadataPath(), []string{"01"})
	if err != nil {
		t.Fatal(err)
	}

	record := loaded.Get("01", metadata.TargetBluesky)
	if record.ErrorKind != errorTransport || record.Error == "" {
		t.Fatalf("failed record = %#v", record)
	}
}

func newThought(t *testing.T, files ...string) archive.Thought {
	t.Helper()

	root, err := archive.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	thought, err := root.Create("thought", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	for index := 0; index+1 < len(files); index += 2 {
		if err := os.WriteFile(filepath.Join(thought.Path(), files[index]), []byte(files[index+1]), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return thought
}
