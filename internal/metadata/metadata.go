// Package metadata models publication state and durable TOML records.
package metadata

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const currentVersion = 1

// Target names supported by the product.
const (
	TargetBluesky = "bluesky"
	TargetX       = "x"
)

// State describes the lifecycle of one post on one target.
type State string

// Publication states.
const (
	StatePending    State = "pending"
	StatePublishing State = "publishing"
	StatePublished  State = "published"
	StateFailed     State = "failed"
	StateRejected   State = "rejected"
)

// Reference identifies a previously published post.
type Reference struct {
	ID  string
	CID string
}

// Record contains the publication history for one post and target.
type Record struct {
	Status      State
	AttemptedAt *time.Time
	PublishedAt *time.Time
	RemoteID    string
	RemoteCID   string
	URL         string
	ParentID    string
	ParentCID   string
	RootID      string
	RootCID     string
	ErrorKind   string
	Error       string
}

// Document is the complete versioned metadata file.
type Document struct {
	Version int
	Posts   map[string]map[string]Record
}

// Targets returns a new slice containing every product target.
func Targets() []string {
	return []string{TargetBluesky, TargetX}
}

// New creates pending metadata for the supplied post names.
func New(posts []string) Document {
	document := Document{
		Version: currentVersion,
		Posts:   make(map[string]map[string]Record),
	}
	document.Ensure(posts, Targets())

	return document
}

// Load reads metadata or returns pending metadata when the file is absent.
func Load(path string, posts []string) (Document, error) {
	data, err := os.ReadFile(path) //nolint:gosec // callers pass paths inside the local thought archive
	if errors.Is(err, os.ErrNotExist) {
		return New(posts), nil
	}

	if err != nil {
		return Document{}, fmt.Errorf("read metadata: %w", err)
	}

	document, err := parse(data)
	if err != nil {
		return Document{}, fmt.Errorf("parse metadata: %w", err)
	}

	document.Ensure(posts, Targets())

	return document, nil
}

// Save writes metadata atomically beside the destination file.
func Save(path string, document Document) error {
	if document.Version == 0 {
		document.Version = currentVersion
	}

	if document.Version != currentVersion {
		return fmt.Errorf("unsupported metadata version %d", document.Version)
	}

	if err := validateDocument(document); err != nil {
		return err
	}

	data, err := marshal(document)
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	directory := filepath.Dir(path)

	file, err := os.CreateTemp(directory, ".meta.toml-*")
	if err != nil {
		return fmt.Errorf("create metadata temporary file: %w", err)
	}

	temporaryPath := file.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
	}()

	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("set metadata permissions: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write metadata: %w", err)
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync metadata: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close metadata: %w", err)
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace metadata: %w", err)
	}

	return nil
}

// Ensure adds missing post and target records as pending.
func (d *Document) Ensure(posts, targets []string) {
	if d.Version == 0 {
		d.Version = currentVersion
	}

	if d.Posts == nil {
		d.Posts = make(map[string]map[string]Record)
	}

	for _, post := range posts {
		if d.Posts[post] == nil {
			d.Posts[post] = make(map[string]Record)
		}

		for _, target := range targets {
			if _, ok := d.Posts[post][target]; ok {
				if d.Posts[post][target].Status == "" {
					record := d.Posts[post][target]
					record.Status = StatePending
					d.Posts[post][target] = record
				}

				continue
			}

			d.Posts[post][target] = Record{Status: StatePending}
		}
	}
}

// Get returns a record, defaulting missing records to pending.
func (d Document) Get(post, target string) Record {
	if targets, ok := d.Posts[post]; ok {
		if record, ok := targets[target]; ok {
			if record.Status == "" {
				record.Status = StatePending
			}

			return record
		}
	}

	return Record{Status: StatePending}
}

// Set stores a record for a post and target.
func (d *Document) Set(post, target string, record Record) error {
	if strings.TrimSpace(post) == "" {
		return errors.New("metadata post name is required")
	}

	if !validTarget(target) {
		return fmt.Errorf("unsupported metadata target %q", target)
	}

	if record.Status == "" {
		record.Status = StatePending
	}

	if !validState(record.Status) {
		return fmt.Errorf("unsupported metadata state %q", record.Status)
	}

	if d.Posts == nil {
		d.Posts = make(map[string]map[string]Record)
	}

	if d.Posts[post] == nil {
		d.Posts[post] = make(map[string]Record)
	}

	d.Posts[post][target] = record

	return nil
}

// MarkPublishing records the start of a new automatic attempt.
func (r *Record) MarkPublishing(at time.Time) error {
	if r.Status == "" {
		r.Status = StatePending
	}

	if r.Status != StatePending && r.Status != StateFailed {
		return fmt.Errorf("cannot start publication from %s", r.Status)
	}

	r.Status = StatePublishing
	r.AttemptedAt = timeValue(at)
	r.ErrorKind = ""
	r.Error = ""

	return nil
}

// MarkPublished records a successful remote publication.
func (r *Record) MarkPublished(at time.Time, result Reference, url string) error {
	if r.Status != StatePublishing {
		return fmt.Errorf("cannot mark %s as published", r.Status)
	}

	if strings.TrimSpace(result.ID) == "" {
		return errors.New("published record requires a remote id")
	}

	r.Status = StatePublished
	r.PublishedAt = timeValue(at)
	r.RemoteID = result.ID
	r.RemoteCID = result.CID
	r.URL = url
	r.ErrorKind = ""
	r.Error = ""

	return nil
}

// MarkFailed records a retryable publication failure.
func (r *Record) MarkFailed(kind, message string) error {
	if r.Status != StatePublishing {
		return fmt.Errorf("cannot mark %s as failed", r.Status)
	}

	r.Status = StateFailed
	r.ErrorKind = strings.TrimSpace(kind)
	r.Error = strings.TrimSpace(message)

	return nil
}

// MarkRejected records a deterministic publication rejection.
func (r *Record) MarkRejected(kind, message string) error {
	if r.Status != StatePublishing {
		return fmt.Errorf("cannot mark %s as rejected", r.Status)
	}

	r.Status = StateRejected
	r.ErrorKind = strings.TrimSpace(kind)
	r.Error = strings.TrimSpace(message)

	return nil
}

// Reply returns the stored parent reference.
func (r Record) Reply() *Reference {
	if r.ParentID == "" {
		return nil
	}

	return &Reference{ID: r.ParentID, CID: r.ParentCID}
}

// RootReply returns the stored root reference.
func (r Record) RootReply() *Reference {
	if r.RootID == "" {
		return nil
	}

	return &Reference{ID: r.RootID, CID: r.RootCID}
}

func parse(data []byte) (Document, error) {
	document := Document{Posts: make(map[string]map[string]Record)}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	currentPost := ""
	currentTarget := ""
	versionFound := false

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++

		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") {
			post, target, err := parseTableLine(&document, line)
			if err != nil {
				return Document{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}

			currentPost = post
			currentTarget = target

			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Document{}, fmt.Errorf("line %d: expected key and value", lineNumber)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if currentPost == "" {
			version, err := parseVersionLine(key, value, lineNumber)
			if err != nil {
				return Document{}, err
			}

			document.Version = version
			versionFound = true

			continue
		}

		record := document.Posts[currentPost][currentTarget]
		if err := parseRecordField(&record, key, value); err != nil {
			return Document{}, fmt.Errorf("line %d: %w", lineNumber, err)
		}

		document.Posts[currentPost][currentTarget] = record
	}

	if err := scanner.Err(); err != nil {
		return Document{}, fmt.Errorf("scan metadata: %w", err)
	}

	if !versionFound {
		return Document{}, errors.New("metadata version is required")
	}

	if document.Version != currentVersion {
		return Document{}, fmt.Errorf("unsupported metadata version %d", document.Version)
	}

	if err := validateDocument(document); err != nil {
		return Document{}, err
	}

	return document, nil
}

func parseTableLine(document *Document, line string) (string, string, error) {
	post, target, err := parseTable(line)
	if err != nil {
		return "", "", err
	}

	if document.Posts[post] == nil {
		document.Posts[post] = make(map[string]Record)
	}

	if _, exists := document.Posts[post][target]; exists {
		return "", "", errors.New("duplicate metadata table")
	}

	document.Posts[post][target] = Record{Status: StatePending}

	return post, target, nil
}

func parseVersionLine(key, value string, lineNumber int) (int, error) {
	if key != "version" {
		return 0, fmt.Errorf("line %d: unexpected top-level key %q", lineNumber, key)
	}

	version, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("line %d: invalid metadata version", lineNumber)
	}

	return version, nil
}

func marshal(document Document) ([]byte, error) {
	posts := make([]string, 0, len(document.Posts))
	for post := range document.Posts {
		posts = append(posts, post)
	}

	sort.Strings(posts)

	var output strings.Builder
	output.WriteString("version = 1\n")

	for _, post := range posts {
		targets := document.Posts[post]

		targetNames := make([]string, 0, len(targets))
		for target := range targets {
			targetNames = append(targetNames, target)
		}

		sort.Strings(targetNames)

		for _, target := range targetNames {
			record := targets[target]
			if record.Status == "" {
				record.Status = StatePending
			}

			fmt.Fprintf(&output, "\n[posts.%q.targets.%s]\n", post, target)
			writeStringField(&output, "status", string(record.Status), true)
			writeTimeField(&output, "attempted_at", record.AttemptedAt)
			writeTimeField(&output, "published_at", record.PublishedAt)
			writeStringField(&output, "remote_id", record.RemoteID, false)
			writeStringField(&output, "remote_cid", record.RemoteCID, false)
			writeStringField(&output, "url", record.URL, false)
			writeStringField(&output, "parent_id", record.ParentID, false)
			writeStringField(&output, "parent_cid", record.ParentCID, false)
			writeStringField(&output, "root_id", record.RootID, false)
			writeStringField(&output, "root_cid", record.RootCID, false)
			writeStringField(&output, "error_kind", record.ErrorKind, false)
			writeStringField(&output, "error", record.Error, false)
		}
	}

	return []byte(output.String()), nil
}

func parseTable(line string) (string, string, error) {
	const prefix = `[posts."`
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "]") {
		return "", "", errors.New("invalid metadata table")
	}

	content := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "]")
	marker := `".targets.`

	index := strings.Index(content, marker)
	if index < 1 {
		return "", "", errors.New("invalid metadata table")
	}

	post := content[:index]

	target := content[index+len(marker):]
	if strings.ContainsAny(post, `"\`) || !validTarget(target) {
		return "", "", errors.New("invalid metadata table")
	}

	return post, target, nil
}

func parseRecordField(record *Record, key, value string) error {
	switch key {
	case "status":
		return parseStatusField(record, value)
	case "attempted_at":
		return parseTimeField(&record.AttemptedAt, "attempted_at", value)
	case "published_at":
		return parseTimeField(&record.PublishedAt, "published_at", value)
	case "remote_id":
		return setString(&record.RemoteID, value)
	case "remote_cid":
		return setString(&record.RemoteCID, value)
	case "url":
		return setString(&record.URL, value)
	case "parent_id":
		return setString(&record.ParentID, value)
	case "parent_cid":
		return setString(&record.ParentCID, value)
	case "root_id":
		return setString(&record.RootID, value)
	case "root_cid":
		return setString(&record.RootCID, value)
	case "error_kind":
		return setString(&record.ErrorKind, value)
	case "error":
		return setString(&record.Error, value)
	default:
		return fmt.Errorf("unsupported metadata field %q", key)
	}
}

func parseStatusField(record *Record, value string) error {
	parsed, err := parseString(value)
	if err != nil {
		return fmt.Errorf("invalid status: %w", err)
	}

	record.Status = State(parsed)
	if !validState(record.Status) {
		return fmt.Errorf("unsupported metadata state %q", parsed)
	}

	return nil
}

func parseTimeField(destination **time.Time, key, value string) error {
	parsed, err := parseTime(value)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", key, err)
	}

	*destination = parsed

	return nil
}

func setString(destination *string, value string) error {
	parsed, err := parseString(value)
	if err != nil {
		return err
	}

	*destination = parsed

	return nil
}

func parseString(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", errors.New("expected a quoted string")
	}

	parsed, err := strconv.Unquote(value)
	if err != nil {
		return "", fmt.Errorf("invalid quoted string: %w", err)
	}

	return parsed, nil
}

func parseTime(value string) (*time.Time, error) {
	parsed, err := parseString(value)
	if err != nil {
		return nil, err
	}

	if parsed == "" {
		return nil, nil
	}

	at, err := time.Parse(time.RFC3339Nano, parsed)
	if err != nil {
		return nil, err
	}

	return timeValue(at), nil
}

func stripComment(line string) string {
	inString := false

	escaped := false
	for index, character := range line {
		if escaped {
			escaped = false
			continue
		}

		if character == '\\' && inString {
			escaped = true
			continue
		}

		if character == '"' {
			inString = !inString
			continue
		}

		if character == '#' && !inString {
			return line[:index]
		}
	}

	return line
}

func writeStringField(output *strings.Builder, key, value string, required bool) {
	if !required && value == "" {
		return
	}

	fmt.Fprintf(output, "%s = %s\n", key, strconv.Quote(value))
}

func writeTimeField(output *strings.Builder, key string, value *time.Time) {
	if value == nil || value.IsZero() {
		return
	}

	writeStringField(output, key, value.UTC().Format(time.RFC3339Nano), true)
}

func validateDocument(document Document) error {
	if document.Version != currentVersion {
		return fmt.Errorf("unsupported metadata version %d", document.Version)
	}

	for post, targets := range document.Posts {
		if strings.TrimSpace(post) == "" {
			return errors.New("metadata post name is required")
		}

		for target, record := range targets {
			if !validTarget(target) {
				return fmt.Errorf("unsupported metadata target %q", target)
			}

			if record.Status == "" {
				record.Status = StatePending
			}

			if !validState(record.Status) {
				return fmt.Errorf("unsupported metadata state %q", record.Status)
			}
		}
	}

	return nil
}

func validTarget(target string) bool {
	return target == TargetBluesky || target == TargetX
}

func validState(state State) bool {
	switch state {
	case StatePending, StatePublishing, StatePublished, StateFailed, StateRejected:
		return true
	default:
		return false
	}
}

func timeValue(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
