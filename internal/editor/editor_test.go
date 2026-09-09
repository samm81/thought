package editor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sourceFile = "post.md"

func TestBuildVimTabs(t *testing.T) {
	t.Parallel()

	invocation, err := Build("nvim --clean", []string{sourceFile, "other.txt"})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"--clean", "-p", sourceFile, "other.txt"}
	if len(invocation.Args) != len(want) {
		t.Fatalf("args = %#v, want %#v", invocation.Args, want)
	}

	for index := range want {
		if invocation.Args[index] != want[index] {
			t.Fatalf("args = %#v, want %#v", invocation.Args, want)
		}
	}
}

func TestBuildNonVim(t *testing.T) {
	t.Parallel()

	invocation, err := Build("emacs --no-window-system", []string{sourceFile})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"--no-window-system", sourceFile}
	if len(invocation.Args) != len(want) {
		t.Fatalf("args = %#v, want %#v", invocation.Args, want)
	}

	for index := range want {
		if invocation.Args[index] != want[index] {
			t.Fatalf("args = %#v, want %#v", invocation.Args, want)
		}
	}
}

func TestOpenReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Open(ctx, "true", []string{sourceFile}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open() error = %v, want %v", err, context.Canceled)
	}
}

func TestOpenUsesEditorFileDirectory(t *testing.T) {
	t.Parallel()

	editorDirectory := t.TempDir()
	editorPath := filepath.Join(editorDirectory, "editor")

	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\npwd > \"${0}.cwd\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(editorPath, 0o700); err != nil { //nolint:gosec // test editor must be executable and is private to t.TempDir
		t.Fatal(err)
	}

	thoughtDirectory := filepath.Join(t.TempDir(), "thought")
	if err := os.Mkdir(thoughtDirectory, 0o750); err != nil {
		t.Fatal(err)
	}

	sourcePath := filepath.Join(thoughtDirectory, sourceFile)
	if err := os.WriteFile(sourcePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Open(context.Background(), editorPath, []string{sourcePath}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(editorPath + ".cwd") //nolint:gosec // path is inside the private test directory
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(string(data)); got != thoughtDirectory {
		t.Fatalf("editor cwd = %q, want %q", got, thoughtDirectory)
	}
}
