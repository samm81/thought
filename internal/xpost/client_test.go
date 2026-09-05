package xpost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type structuredRequest struct {
	Operation   string       `json:"operation"`
	Target      string       `json:"target"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
	ReplyTo     *Reference   `json:"reply_to"`
	RootReplyTo *Reference   `json:"root_reply_to"`
}

const clientTestMessage = "hello"

func TestPublishDecodesResult(t *testing.T) {
	t.Parallel()

	command := helperCommand(t, "printf '%s\\n' '{\"status\":\"published\",\"remote_id\":\"post-1\"}'")
	client := New(command, time.Second)

	response, err := client.Publish(context.Background(), Request{Target: "bluesky", Text: clientTestMessage})
	if err != nil {
		t.Fatal(err)
	}

	if response.Status != "published" || response.RemoteID != "post-1" {
		t.Fatalf("response = %#v", response)
	}
}

func TestPublishRejectsMalformedResult(t *testing.T) {
	t.Parallel()

	command := helperCommand(t, "printf '%s\\n' '{\"status\":\"published\"}'")

	client := New(command, time.Second)
	if _, err := client.Publish(context.Background(), Request{}); err == nil {
		t.Fatal("Publish() error = nil, want missing remote id")
	}
}

func TestPublishHonorsTimeout(t *testing.T) {
	t.Parallel()

	command := helperCommand(t, "sleep 1")

	client := New(command, 10*time.Millisecond)
	if _, err := client.Publish(context.Background(), Request{}); err == nil {
		t.Fatal("Publish() error = nil, want timeout")
	}
}

func TestPublishSendsStructuredRequest(t *testing.T) {
	t.Parallel()

	capturePath := filepath.Join(t.TempDir(), "request.json")
	command := helperCommand(t, fmt.Sprintf("cat > %q\nprintf '%%s\\n' '{\"status\":\"published\",\"remote_id\":\"post-1\"}'", capturePath))

	request := Request{
		Target: "x",
		Text:   clientTestMessage,
		Attachments: []Attachment{{
			Path: "image.png",
			Alt:  "a result",
		}},
		ReplyTo:     &Reference{ID: "parent-1", CID: "parent-cid"},
		RootReplyTo: &Reference{ID: "root-1", CID: "root-cid"},
	}
	if _, err := New(command, time.Second).Publish(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(capturePath) //nolint:gosec // test path is created under t.TempDir
	if err != nil {
		t.Fatal(err)
	}

	var got structuredRequest
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("request = %q: %v", payload, err)
	}

	structuredRequestAssert(t, got, "publish")
}

func TestValidateSendsValidationOperation(t *testing.T) {
	t.Parallel()

	capturePath := filepath.Join(t.TempDir(), "request.json")
	command := helperCommand(t, fmt.Sprintf("cat > %q\nprintf '%%s\\n' '{\"status\":\"validated\"}'", capturePath))
	request := Request{
		Target: "x",
		Text:   clientTestMessage,
		Attachments: []Attachment{{
			Path: "image.png",
			Alt:  "a result",
		}},
		ReplyTo:     &Reference{ID: "parent-1", CID: "parent-cid"},
		RootReplyTo: &Reference{ID: "root-1", CID: "root-cid"},
	}

	if err := New(command, time.Second).Validate(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(capturePath) //nolint:gosec // test path is created under t.TempDir
	if err != nil {
		t.Fatal(err)
	}

	var got structuredRequest
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("request = %q: %v", payload, err)
	}

	structuredRequestAssert(t, got, "validate")
}

func TestValidateRejectsRejectedResult(t *testing.T) {
	t.Parallel()

	command := helperCommand(t, "printf '%s\\n' '{\"status\":\"rejected\",\"error\":\"message too long\"}'")

	err := New(command, time.Second).Validate(context.Background(), Request{})
	if err == nil || !strings.Contains(err.Error(), "message too long") {
		t.Fatalf("Validate() error = %v, want rejection", err)
	}
}

func structuredRequestAssert(t *testing.T, got structuredRequest, operation string) {
	t.Helper()

	if got.Operation != operation || got.Target != "x" || got.Text != clientTestMessage {
		t.Fatalf("request = %#v", got)
	}

	if len(got.Attachments) != 1 || got.Attachments[0].Path != "image.png" || got.Attachments[0].Alt != "a result" {
		t.Fatalf("attachments = %#v", got.Attachments)
	}

	if got.ReplyTo == nil || got.ReplyTo.ID != "parent-1" || got.RootReplyTo == nil || got.RootReplyTo.ID != "root-1" {
		t.Fatalf("references = %#v / %#v", got.ReplyTo, got.RootReplyTo)
	}
}

func helperCommand(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xpost-helper")

	content := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // test bridge must be executable and is private to t.TempDir
		t.Fatal(err)
	}

	return path
}
