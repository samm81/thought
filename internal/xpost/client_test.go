package xpost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPublishDecodesResult(t *testing.T) {
	t.Parallel()

	command := helperCommand(t, "printf '%s\\n' '{\"status\":\"published\",\"remote_id\":\"post-1\"}'")
	client := New(command, time.Second)
	response, err := client.Publish(context.Background(), Request{Target: "bluesky", Text: "hello"})
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

func helperCommand(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xpost-helper")
	content := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
