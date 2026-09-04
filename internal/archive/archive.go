// Package archive owns local thought path resolution and file discovery.
package archive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultArchiveDirectory = "thoughts"
	firstPostName           = "01.md"
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

// Post identifies one numbered Markdown file.
type Post struct {
	Number int
	Path   string
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

		postPath := filepath.Join(candidatePath, firstPostName)

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

// Posts discovers all contiguous numbered Markdown files in the thought.
func (t Thought) Posts() ([]Post, error) {
	entries, err := os.ReadDir(t.path)
	if err != nil {
		return nil, fmt.Errorf("read thought directory: %w", err)
	}

	posts := make(map[int]Post)

	for _, entry := range entries {
		post, ok, err := postFromEntry(t.path, entry)
		if err != nil {
			return nil, err
		}

		if !ok {
			continue
		}

		if _, exists := posts[post.Number]; exists {
			return nil, fmt.Errorf("duplicate post number %02d", post.Number)
		}

		posts[post.Number] = post
	}

	return sequencePosts(posts)
}

func postFromEntry(thoughtPath string, entry os.DirEntry) (Post, bool, error) {
	name := entry.Name()
	if !strings.HasSuffix(name, ".md") {
		return Post{}, false, nil
	}

	stem := strings.TrimSuffix(name, ".md")
	if !allDigits(stem) {
		return Post{}, false, nil
	}

	if len(stem) != 2 {
		return Post{}, false, fmt.Errorf("invalid post filename %q: use two digits", name)
	}

	number, err := strconv.Atoi(stem)
	if err != nil || number < 1 {
		return Post{}, false, fmt.Errorf("invalid post filename %q", name)
	}

	info, err := entry.Info()
	if err != nil {
		return Post{}, false, fmt.Errorf("inspect post %q: %w", name, err)
	}

	if !info.Mode().IsRegular() {
		return Post{}, false, fmt.Errorf("post %q is not a regular file", name)
	}

	return Post{
		Number: number,
		Path:   filepath.Join(thoughtPath, name),
	}, true, nil
}

func sequencePosts(posts map[int]Post) ([]Post, error) {
	if len(posts) == 0 {
		return nil, errors.New("no numbered posts found")
	}

	if _, ok := posts[1]; !ok {
		return nil, errors.New("post sequence must start at 01.md")
	}

	numbers := make([]int, 0, len(posts))
	for number := range posts {
		numbers = append(numbers, number)
	}

	sort.Ints(numbers)

	result := make([]Post, 0, len(numbers))
	for index, number := range numbers {
		expected := index + 1
		if number != expected {
			return nil, fmt.Errorf("post sequence skips %02d.md", expected)
		}

		result = append(result, posts[number])
	}

	return result, nil
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

func allDigits(value string) bool {
	if value == "" {
		return false
	}

	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}

	return true
}
