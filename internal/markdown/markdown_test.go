package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTrailingAttachments(t *testing.T) {
	t.Parallel()

	document, err := Parse([]byte("# hello\n\nsee [the result](https://example.com).\n\n![first view](one.png)\n![second view](two.png)\n"))
	if err != nil {
		t.Fatal(err)
	}
	if document.Text != "hello\n\nsee the result (https://example.com)." {
		t.Fatalf("Text = %q", document.Text)
	}
	if len(document.Attachments) != 2 || document.Attachments[1].Alt != "second view" {
		t.Fatalf("Attachments = %#v", document.Attachments)
	}
}

func TestParseRejectsInlineAndNonTrailingImages(t *testing.T) {
	t.Parallel()

	for _, source := range []string{
		"text ![image](one.png)",
		"![image](one.png)\n\nmore text",
	} {
		if _, err := Parse([]byte(source)); err == nil {
			t.Errorf("Parse(%q) error = nil", source)
		}
	}
}

func TestResolveAttachmentSafety(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inside := filepath.Join(root, "image.png")
	if err := os.WriteFile(inside, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveAttachment(root, "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != inside {
		t.Fatalf("resolved = %q, want %q", resolved, inside)
	}

	for _, path := range []string{"../image.png", "/tmp/image.png", "missing.png", `windows\\image.png`} {
		if _, err := ResolveAttachment(root, path); err == nil {
			t.Errorf("ResolveAttachment(%q) error = nil", path)
		}
	}

	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.png")
	if err := os.Symlink(outside, link); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not supported") {
			return
		}
		t.Fatal(err)
	}
	if _, err := ResolveAttachment(root, "link.png"); err == nil {
		t.Fatal("ResolveAttachment() error = nil for escaping symlink")
	}
}
