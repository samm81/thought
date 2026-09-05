package editor

import (
	"context"
	"errors"
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
