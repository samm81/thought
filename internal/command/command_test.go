package command

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samm81/thought/internal/archive"
	"github.com/samm81/thought/internal/metadata"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		arguments     []string
		wantOutput    string
		wantError     error
		wantErrorText string
	}{
		{
			name:       "no arguments",
			wantOutput: usageText,
		},
		{
			name:       "long help",
			arguments:  []string{"--help"},
			wantOutput: usageText,
		},
		{
			name:       "short help",
			arguments:  []string{"-h"},
			wantOutput: usageText,
		},
		{
			name:          "unknown command",
			arguments:     []string{"unknown"},
			wantError:     ErrUsage,
			wantErrorText: "usage:",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			err := Run(t.Context(), test.arguments, &output)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Run() error = %v, want %v", err, test.wantError)
			}
			if test.wantErrorText != "" && (err == nil || !strings.Contains(err.Error(), test.wantErrorText)) {
				t.Fatalf("Run() error = %q, want it to contain %q", err, test.wantErrorText)
			}

			if test.wantOutput == usageText && output.String() != test.wantOutput {
				t.Fatalf("Run() output = %q, want %q", output.String(), test.wantOutput)
			}
			if test.wantOutput != usageText && !strings.Contains(output.String(), test.wantOutput) {
				t.Fatalf("Run() output = %q, want it to contain %q", output.String(), test.wantOutput)
			}
		})
	}
}

func TestRunRejectsInvalidPublishArguments(t *testing.T) {
	var output bytes.Buffer
	err := Run(context.Background(), []string{"publish"}, &output)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("Run() error = %v, want usage error", err)
	}
}

func TestRunNewCreatesThought(t *testing.T) {
	archiveRoot := t.TempDir()
	t.Setenv("THOUGHT_HOME", archiveRoot)
	t.Setenv("VISUAL", "true")
	t.Setenv("EDITOR", "true")

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"new", "demo"}, &output); err != nil {
		t.Fatal(err)
	}
	thoughtPath := strings.TrimSpace(output.String())
	for _, path := range []string{
		filepath.Join(thoughtPath, "01.md"),
		filepath.Join(thoughtPath, "meta.toml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("created path %q: %v", path, err)
		}
	}
}

func TestRunStatusReportsMetadataAndRecovery(t *testing.T) {
	archiveRoot := t.TempDir()
	root, err := archive.New(archiveRoot)
	if err != nil {
		t.Fatal(err)
	}
	thought, err := root.Create("demo", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	publication := metadata.New([]string{"01"})
	attemptedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	published := publication.Get("01", metadata.TargetBluesky)
	if err := published.MarkPublishing(attemptedAt); err != nil {
		t.Fatal(err)
	}
	if err := published.MarkPublished(attemptedAt.Add(time.Second), metadata.Reference{ID: "post-1", CID: "cid-1"}, "https://bsky.app/post/1"); err != nil {
		t.Fatal(err)
	}
	if err := publication.Set("01", metadata.TargetBluesky, published); err != nil {
		t.Fatal(err)
	}
	rejected := publication.Get("01", metadata.TargetX)
	if err := rejected.MarkPublishing(attemptedAt); err != nil {
		t.Fatal(err)
	}
	if err := rejected.MarkRejected("validation", "too long"); err != nil {
		t.Fatal(err)
	}
	if err := publication.Set("01", metadata.TargetX, rejected); err != nil {
		t.Fatal(err)
	}
	if err := metadata.Save(thought.MetadataPath(), publication); err != nil {
		t.Fatal(err)
	}

	t.Setenv("THOUGHT_HOME", archiveRoot)
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"status", "demo"}, &output); err != nil {
		t.Fatal(err)
	}
	status := output.String()
	for _, value := range []string{
		`01 bluesky: published`,
		`remote_id="post-1"`,
		`remote_cid="cid-1"`,
		`url="https://bsky.app/post/1"`,
		`01 x: rejected`,
		`error="too long"`,
		`action="fix the post and edit meta.toml status to pending"`,
	} {
		if !strings.Contains(status, value) {
			t.Fatalf("status = %q, want %q", status, value)
		}
	}
}

func TestRunPublishTargetSelection(t *testing.T) {
	archiveRoot := t.TempDir()
	root, err := archive.New(archiveRoot)
	if err != nil {
		t.Fatal(err)
	}
	thought, err := root.Create("demo", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bridgePath := filepath.Join(t.TempDir(), "xpost")
	if err := os.WriteFile(bridgePath, []byte("#!/bin/sh\nprintf '%s\\n' '{\"status\":\"published\",\"remote_id\":\"post-1\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THOUGHT_HOME", archiveRoot)
	t.Setenv("THOUGHT_XPOST", bridgePath)

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"publish", thought.Name(), "--target", "x"}, &output); err != nil {
		t.Fatal(err)
	}
	publication, err := metadata.Load(thought.MetadataPath(), []string{"01"})
	if err != nil {
		t.Fatal(err)
	}
	if got := publication.Get("01", metadata.TargetX).Status; got != metadata.StatePublished {
		t.Fatalf("X state = %q, want published", got)
	}
	if got := publication.Get("01", metadata.TargetBluesky).Status; got != metadata.StatePending {
		t.Fatalf("Bluesky state = %q, want pending", got)
	}
	if !strings.Contains(output.String(), "x 01: published") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", want: 0},
		{name: "usage", err: ErrUsage, want: 2},
		{name: "canceled", err: context.Canceled, want: 130},
		{name: "deadline", err: context.DeadlineExceeded, want: 124},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := ExitCode(test.err); got != test.want {
				t.Fatalf("ExitCode(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}
