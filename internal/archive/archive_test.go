package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAndPosts(t *testing.T) {
	t.Parallel()

	root, err := New(filepath.Join(t.TempDir(), "thoughts"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	thought, err := root.Create("release-note", at)
	if err != nil {
		t.Fatal(err)
	}

	if thought.Name() != "release-note" {
		t.Fatalf("Name() = %q, want release-note", thought.Name())
	}
	if _, err := os.Stat(filepath.Join(thought.Path(), "01.md")); err != nil {
		t.Fatalf("initial post: %v", err)
	}
	posts, err := thought.Posts()
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].Number != 1 {
		t.Fatalf("Posts() = %#v, want one post numbered 1", posts)
	}
}

func TestCreateGeneratedName(t *testing.T) {
	t.Parallel()

	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	first, err := root.Create("", at)
	if err != nil {
		t.Fatal(err)
	}
	second, err := root.Create("", at)
	if err != nil {
		t.Fatal(err)
	}
	if first.Name() != "20260102-030405" || second.Name() != "20260102-030405-1" {
		t.Fatalf("generated names = %q, %q", first.Name(), second.Name())
	}
}

func TestPostsRejectInvalidSequence(t *testing.T) {
	t.Parallel()

	rootDirectory := t.TempDir()
	directory := filepath.Join(rootDirectory, "thought")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01.md", "03.md"} {
		if err := os.WriteFile(filepath.Join(directory, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := New(rootDirectory)
	if err != nil {
		t.Fatal(err)
	}
	thought, err := root.Thought(filepath.Base(directory))
	if err != nil {
		t.Fatal(err)
	}
	_, err = thought.Posts()
	if err == nil {
		t.Fatal("Posts() error = nil, want sequence error")
	}
}

func TestThoughtNameSafety(t *testing.T) {
	t.Parallel()

	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", " ", "..", "../escape", "/absolute", `windows\\escape`} {
		if _, err := root.Thought(name); err == nil {
			t.Errorf("Thought(%q) error = nil", name)
		}
	}
}

func TestCreateRejectsWhitespaceName(t *testing.T) {
	t.Parallel()

	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Create(" ", time.Now()); err == nil {
		t.Fatal("Create() error = nil, want invalid name")
	}
}
