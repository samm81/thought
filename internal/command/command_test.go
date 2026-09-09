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

const publishCommand = "publish"

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
		{
			name:          "edit accepts at most one thought",
			arguments:     []string{"edit", "one", "two"},
			wantError:     ErrUsage,
			wantErrorText: "edit accepts at most one thought name or directory",
		},
		{
			name:          "status accepts at most one thought",
			arguments:     []string{"status", "one", "two"},
			wantError:     ErrUsage,
			wantErrorText: "status accepts at most one thought name or directory",
		},
		{
			name:          "completion requires a supported shell",
			arguments:     []string{"completion", "bash"},
			wantError:     ErrUsage,
			wantErrorText: "unsupported completion shell",
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
	t.Parallel()

	var output bytes.Buffer

	err := Run(context.Background(), []string{publishCommand, "one", "two"}, &output)
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
		filepath.Join(thoughtPath, "post.md"),
		filepath.Join(thoughtPath, "meta.toml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("created path %q: %v", path, err)
		}
	}
}

func TestRunEditDefaultsToMostRecentThought(t *testing.T) {
	archiveRoot := t.TempDir()

	root, err := archive.New(archiveRoot)
	if err != nil {
		t.Fatal(err)
	}

	_, newest := recentThoughts(t, root)
	editorPath := testEditor(t)

	t.Setenv("THOUGHT_HOME", archiveRoot)
	t.Setenv("VISUAL", editorPath)

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"edit"}, &output); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(editorPath + ".arg") //nolint:gosec // path is inside the private test directory
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(newest.Path(), "post.md")
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("edited source = %q, want %q", got, want)
	}
}

func TestRunCompleteListsThoughts(t *testing.T) {
	archiveRoot := t.TempDir()

	root, err := archive.New(archiveRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"alpha", "beta", "release-note"} {
		if _, err := root.Create(name, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("THOUGHT_HOME", archiveRoot)

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"__complete", "re"}, &output); err != nil {
		t.Fatal(err)
	}

	if got, want := output.String(), "release-note\n"; got != want {
		t.Fatalf("completion = %q, want %q", got, want)
	}
}

func TestRunCompletionZsh(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"completion", "zsh"}, &output); err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{
		"#compdef thought",
		"command thought __complete",
		"compdef _thought thought",
	} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("completion = %q, want %q", output.String(), value)
		}
	}
}

func TestRunStatusDefaultsToMostRecentThought(t *testing.T) {
	archiveRoot := t.TempDir()

	root, err := archive.New(archiveRoot)
	if err != nil {
		t.Fatal(err)
	}

	older, newest := recentThoughts(t, root)
	for _, thought := range []archive.Thought{older, newest} {
		if err := metadata.Save(thought.MetadataPath(), metadata.New([]string{"01"})); err != nil {
			t.Fatal(err)
		}
	}

	oldAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	newAt := oldAt.Add(time.Minute)
	if err := os.Chtimes(older.Path(), oldAt, oldAt); err != nil {
		t.Fatal(err)
	}

	if err := os.Chtimes(newest.Path(), newAt, newAt); err != nil {
		t.Fatal(err)
	}

	t.Setenv("THOUGHT_HOME", archiveRoot)

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"status"}, &output); err != nil {
		t.Fatal(err)
	}

	if want := "thought: " + newest.Name() + "\n"; !strings.Contains(output.String(), want) {
		t.Fatalf("status = %q, want %q", output.String(), want)
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

	bridgePath := testBridge(t)

	t.Setenv("THOUGHT_HOME", archiveRoot)
	t.Setenv("THOUGHT_XPOST", bridgePath)

	var output bytes.Buffer
	if err := Run(context.Background(), []string{publishCommand, thought.Path(), "--target", "x"}, &output); err != nil {
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

func TestRunPublishSelectsMostRecentThoughtAfterConfirmation(t *testing.T) {
	archiveRoot := t.TempDir()

	root, err := archive.New(archiveRoot)
	if err != nil {
		t.Fatal(err)
	}

	older, newer := recentThoughts(t, root)
	bridgePath := testBridge(t)

	t.Setenv("THOUGHT_HOME", archiveRoot)
	t.Setenv("THOUGHT_XPOST", bridgePath)

	var output bytes.Buffer
	if err := run(context.Background(), []string{publishCommand}, strings.NewReader("y\n"), &output); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), `publish "newer" to bluesky and x? [y/N]`) {
		t.Fatalf("output = %q, want confirmation for newer thought", output.String())
	}

	publication, err := metadata.Load(newer.MetadataPath(), []string{"01"})
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range metadata.Targets() {
		if got := publication.Get("01", target).Status; got != metadata.StatePublished {
			t.Fatalf("%s state = %q, want published", target, got)
		}
	}

	if _, err := os.Stat(older.MetadataPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("older metadata stat error = %v, want absent metadata", err)
	}
}

func testBridge(t *testing.T) string {
	t.Helper()

	bridgePath := filepath.Join(t.TempDir(), "xpost")
	bridge := "#!/bin/sh\nrequest=\"$(cat)\"\ncase \"$request\" in\n  *'\"operation\":\"validate\"'*) printf '%s\\n' '{\"status\":\"validated\"}' ;;\n  *) printf '%s\\n' '{\"status\":\"published\",\"remote_id\":\"post-1\"}' ;;\nesac\n"

	if err := os.WriteFile(bridgePath, []byte(bridge), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(bridgePath, 0o700); err != nil { //nolint:gosec // test bridge must be executable and is private to t.TempDir
		t.Fatal(err)
	}

	return bridgePath
}

func testEditor(t *testing.T) string {
	t.Helper()

	editorPath := filepath.Join(t.TempDir(), "editor")
	editor := "#!/bin/sh\nprintf '%s\\n' \"$1\" > \"${0}.arg\"\n"

	if err := os.WriteFile(editorPath, []byte(editor), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(editorPath, 0o700); err != nil { //nolint:gosec // test editor must be executable and is private to t.TempDir
		t.Fatal(err)
	}

	return editorPath
}

func recentThoughts(t *testing.T, root archive.Root) (archive.Thought, archive.Thought) {
	t.Helper()

	older, err := root.Create("older", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	newer, err := root.Create("newer", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct {
		thought archive.Thought
		content string
	}{
		{thought: older, content: "old"},
		{thought: newer, content: "new"},
	} {
		if err := os.WriteFile(filepath.Join(item.thought.Path(), "post.md"), []byte(item.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	oldAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	newAt := oldAt.Add(time.Minute)
	if err := os.Chtimes(older.Path(), oldAt, oldAt); err != nil {
		t.Fatal(err)
	}

	if err := os.Chtimes(newer.Path(), newAt, newAt); err != nil {
		t.Fatal(err)
	}

	return older, newer
}

func TestRunPublishDeclinesMostRecentThought(t *testing.T) {
	archiveRoot := t.TempDir()

	root, err := archive.New(archiveRoot)
	if err != nil {
		t.Fatal(err)
	}

	thought, err := root.Create("demo", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("THOUGHT_HOME", archiveRoot)

	var output bytes.Buffer
	if err := run(context.Background(), []string{publishCommand}, strings.NewReader("n\n"), &output); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), `publish "demo" to bluesky and x? [y/N]`) || !strings.Contains(output.String(), "publication cancelled") {
		t.Fatalf("output = %q, want declined confirmation", output.String())
	}

	if _, err := os.Stat(thought.MetadataPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("metadata stat error = %v, want absent metadata", err)
	}
}

func TestRunPublishRejectsAlreadyPublishedMostRecentThought(t *testing.T) {
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

	t.Setenv("THOUGHT_HOME", archiveRoot)

	var output bytes.Buffer

	err = run(context.Background(), []string{publishCommand}, strings.NewReader("y\n"), &output)
	if err == nil || !strings.Contains(err.Error(), "already published") {
		t.Fatalf("run() error = %v, want already published error", err)
	}

	if strings.Contains(output.String(), "[y/N]") {
		t.Fatalf("output = %q, want no confirmation for published thought", output.String())
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
