package publish

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samm81/thought/internal/archive"
	"github.com/samm81/thought/internal/metadata"
	"github.com/samm81/thought/internal/xpost"
)

type fakeClient struct {
	responses []xpost.Response
	requests  []xpost.Request
}

func (c *fakeClient) Publish(_ context.Context, request xpost.Request) (xpost.Response, error) {
	c.requests = append(c.requests, request)
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

func TestPublishThreadsAndRecordsReferences(t *testing.T) {
	t.Parallel()

	thought := newThought(t, "01.md", "first", "02.md", "second")
	client := &fakeClient{responses: []xpost.Response{
		{Status: "published", RemoteID: "at://first", RemoteCID: "cid-first"},
		{Status: "published", RemoteID: "at://second", RemoteCID: "cid-second"},
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
		{Status: "published", RemoteID: "at://first", RemoteCID: "cid-first"},
		{Status: "failed", ErrorKind: "transport", Error: "timeout"},
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
		{Status: "failed", ErrorKind: "transport", Error: "network"},
		{Status: "published", RemoteID: "x-1"},
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
	for index := 0; index < len(files); index += 2 {
		if err := os.WriteFile(filepath.Join(thought.Path(), files[index]), []byte(files[index+1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return thought
}
