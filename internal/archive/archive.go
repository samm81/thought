// Package archive owns local thought path resolution and file discovery.
package archive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultArchiveDirectory = "thoughts"
	sourceFileName          = "post.md"
)

// Root identifies the directory containing all thoughts.
type Root struct {
	path string
}

// Thought identifies one local thought directory.
type Thought struct {
	name string
	path string
}

// Source contains the authored Markdown source for one thought.
type Source struct {
	Path string
	Data []byte
}

// FromEnvironment resolves THOUGHT_HOME or the default archive location.
func FromEnvironment() (Root, error) {
	path := strings.TrimSpace(os.Getenv("THOUGHT_HOME"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Root{}, fmt.Errorf("resolve home directory: %w", err)
		}

		path = filepath.Join(home, defaultArchiveDirectory)
	}

	path, err := expandHome(path)
	if err != nil {
		return Root{}, err
	}

	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return Root{}, fmt.Errorf("resolve archive root: %w", err)
	}

	return Root{path: path}, nil
}

// New constructs a root from an explicit path.
func New(path string) (Root, error) {
	if strings.TrimSpace(path) == "" {
		return Root{}, errors.New("archive root is required")
	}

	path, err := expandHome(path)
	if err != nil {
		return Root{}, err
	}

	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return Root{}, fmt.Errorf("resolve archive root: %w", err)
	}

	return Root{path: path}, nil
}

// Path returns the absolute archive root path.
func (r Root) Path() string {
	return r.path
}

// Thought resolves a thought name or full directory path under the archive root.
func (r Root) Thought(value string) (Thought, error) {
	path, err := r.resolveThoughtPath(value)
	if err != nil {
		return Thought{}, err
	}

	return Thought{
		name: filepath.Base(path),
		path: path,
	}, nil
}

// MostRecentThought returns the newest direct child thought in the archive.
func (r Root) MostRecentThought() (Thought, error) {
	entries, err := os.ReadDir(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Thought{}, errors.New("no thoughts found")
		}

		return Thought{}, fmt.Errorf("read archive root: %w", err)
	}

	var (
		recent   Thought
		recentAt time.Time
	)

	found := false

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return Thought{}, fmt.Errorf("inspect thought %q: %w", entry.Name(), err)
		}

		if found && (info.ModTime().Before(recentAt) || info.ModTime().Equal(recentAt) && entry.Name() < recent.name) {
			continue
		}

		recent = Thought{
			name: entry.Name(),
			path: filepath.Join(r.path, entry.Name()),
		}
		recentAt = info.ModTime()
		found = true
	}

	if !found {
		return Thought{}, errors.New("no thoughts found")
	}

	return recent, nil
}

// Create creates a thought directory and its initial Markdown file.
func (r Root) Create(name string, at time.Time) (Thought, error) {
	if err := os.MkdirAll(r.path, 0o750); err != nil {
		return Thought{}, fmt.Errorf("create archive root: %w", err)
	}

	generated := name == ""

	baseName := name
	if generated {
		baseName = at.Format("20060102-150405")
	}

	if err := validateName(baseName); err != nil {
		return Thought{}, err
	}

	for suffix := 0; ; suffix++ {
		candidateName := baseName
		if generated && suffix > 0 {
			candidateName = fmt.Sprintf("%s-%d", baseName, suffix)
		}

		candidatePath := filepath.Join(r.path, candidateName)

		err := os.Mkdir(candidatePath, 0o750)
		if errors.Is(err, os.ErrExist) && generated {
			continue
		}

		if err != nil {
			return Thought{}, fmt.Errorf("create thought %q: %w", candidateName, err)
		}

		postPath := filepath.Join(candidatePath, sourceFileName)

		file, err := os.OpenFile(postPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // postPath is built from the validated archive root and name
		if err != nil {
			return Thought{}, fmt.Errorf("create initial post: %w", err)
		}

		if err := file.Close(); err != nil {
			return Thought{}, fmt.Errorf("close initial post: %w", err)
		}

		return Thought{name: candidateName, path: candidatePath}, nil
	}
}

// Name returns the thought directory name.
func (t Thought) Name() string {
	return t.name
}

// Path returns the thought directory path.
func (t Thought) Path() string {
	return t.path
}

// MetadataPath returns the path to the thought's metadata file.
func (t Thought) MetadataPath() string {
	return filepath.Join(t.path, "meta.toml")
}

// ReadSource reads the single authored Markdown source for the thought.
func (t Thought) ReadSource() (Source, error) {
	entries, err := os.ReadDir(t.path)
	if err != nil {
		return Source{}, fmt.Errorf("read thought directory: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == sourceFileName {
			continue
		}

		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return Source{}, fmt.Errorf("unsupported Markdown source %q; use post.md with --- separators", entry.Name())
		}
	}

	path := filepath.Join(t.path, sourceFileName)

	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Source{}, errors.New("post.md is required")
		}

		return Source{}, fmt.Errorf("inspect post.md: %w", err)
	}

	if !info.Mode().IsRegular() {
		return Source{}, errors.New("post.md is not a regular file")
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is inside the validated thought archive
	if err != nil {
		return Source{}, fmt.Errorf("read post.md: %w", err)
	}

	return Source{Path: path, Data: data}, nil
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	if path == "~" {
		return home, nil
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func (r Root) resolveThoughtPath(value string) (string, error) {
	if r.path == "" {
		return "", errors.New("archive root is required")
	}

	if value == "" || strings.ContainsRune(value, 0) {
		return "", errors.New("thought name or directory is required")
	}

	path, err := expandHome(value)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(r.path, path)
	}

	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve thought directory: %w", err)
	}

	relative, err := filepath.Rel(r.path, path)
	if err != nil {
		return "", fmt.Errorf("resolve thought directory: %w", err)
	}

	if !isDirectChild(relative) {
		return "", fmt.Errorf("thought directory %q must be inside the archive root", value)
	}

	if err := validateName(filepath.Base(path)); err != nil {
		return "", err
	}

	return path, nil
}

func isDirectChild(relative string) bool {
	if relative == "." || relative == ".." {
		return false
	}

	if strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false
	}

	return !strings.Contains(relative, string(filepath.Separator))
}

func validateName(name string) error {
	if name == "" || strings.TrimSpace(name) != name || name == "." || name == ".." {
		return errors.New("thought name is invalid")
	}

	if strings.ContainsRune(name, 0) || strings.ContainsAny(name, `/\\`) || filepath.IsAbs(name) {
		return fmt.Errorf("thought name %q must be a single relative directory name", name)
	}

	return nil
}
