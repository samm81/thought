package metadata

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "meta.toml")

	document := New([]string{"01", "02"})
	document.SourceHash = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	record := document.Get("01", TargetBluesky)

	attemptedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := record.MarkPublishing(attemptedAt); err != nil {
		t.Fatal(err)
	}

	record.ParentID = "at://parent"
	record.ParentCID = "cid-parent"
	record.RootID = "at://root"

	record.RootCID = "cid-root"
	if err := record.MarkPublished(attemptedAt.Add(time.Second), Reference{ID: "at://post", CID: "cid-post"}, "https://bsky.app/post"); err != nil {
		t.Fatal(err)
	}

	if err := document.Set("01", TargetBluesky, record); err != nil {
		t.Fatal(err)
	}

	if err := Save(path, document); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path, []string{"01", "02", "03"})
	if err != nil {
		t.Fatal(err)
	}

	got := loaded.Get("01", TargetBluesky)
	if got.Status != StatePublished || got.RemoteID != "at://post" || got.RemoteCID != "cid-post" {
		t.Fatalf("loaded record = %#v", got)
	}

	if got.Reply() == nil || got.Reply().ID != "at://parent" {
		t.Fatalf("loaded parent = %#v", got.Reply())
	}

	if loaded.SourceHash != document.SourceHash {
		t.Fatalf("source hash = %q, want %q", loaded.SourceHash, document.SourceHash)
	}

	if loaded.Get("03", TargetX).Status != StatePending {
		t.Fatalf("missing record status = %q, want pending", loaded.Get("03", TargetX).Status)
	}
}

func TestLoadMissingIsPending(t *testing.T) {
	t.Parallel()

	document, err := Load(filepath.Join(t.TempDir(), "missing.toml"), []string{"01"})
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range Targets() {
		if got := document.Get("01", target).Status; got != StatePending {
			t.Fatalf("status for %s = %q, want pending", target, got)
		}
	}
}

func TestStateTransitions(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	record := Record{Status: StatePending}
	if err := record.MarkPublishing(at); err != nil {
		t.Fatal(err)
	}

	if err := record.MarkFailed("transport", "timeout"); err != nil {
		t.Fatal(err)
	}

	if record.Status != StateFailed || record.Error != "timeout" {
		t.Fatalf("failed record = %#v", record)
	}

	if err := record.MarkPublishing(at); err != nil {
		t.Fatal(err)
	}

	if err := record.MarkRejected("validation", "too long"); err != nil {
		t.Fatal(err)
	}

	if err := record.MarkPublishing(at); err == nil {
		t.Fatal("MarkPublishing() error = nil for rejected record")
	}

	if err := record.MarkPublished(at, Reference{}, ""); err == nil {
		t.Fatal("MarkPublished() error = nil for rejected record")
	}
}

func TestLoadRejectsMalformedMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "meta.toml")
	if err := os.WriteFile(path, []byte("version = 1\n\n[posts.\"01\".targets.bluesky]\nstatus = \"unknown\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path, []string{"01"}); err == nil {
		t.Fatal("Load() error = nil for invalid state")
	}
}

func TestLoadRejectsInvalidSourceHash(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "meta.toml")
	if err := os.WriteFile(path, []byte("version = 1\nsource_hash = \"sha256:not-a-digest\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path, []string{"01"}); err == nil {
		t.Fatal("Load() error = nil, want invalid source hash")
	}
}
