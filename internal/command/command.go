// Package command parses and executes the thought command-line interface.
package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samm81/thought/internal/archive"
	"github.com/samm81/thought/internal/editor"
	"github.com/samm81/thought/internal/metadata"
	"github.com/samm81/thought/internal/publish"
	"github.com/samm81/thought/internal/xpost"
)

var (
	// ErrUsage indicates that command-line input is invalid.
	ErrUsage = errors.New("usage error")
)

const usageText = `usage: thought <command> [arguments]

commands:
  new [name]                      create and edit a thought
  edit <name-or-directory>        edit a thought
  publish [name-or-directory] [--target ...]   publish a thought
  status <name-or-directory>      show local publication state

options:
  -h, --help                     show this help
`

// Run parses arguments and executes one command.
func Run(ctx context.Context, arguments []string, output io.Writer) error {
	return run(ctx, arguments, os.Stdin, output)
}

func run(ctx context.Context, arguments []string, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(arguments) == 0 || arguments[0] == "-h" || arguments[0] == "--help" || arguments[0] == "help" {
		return writeUsage(output)
	}

	switch arguments[0] {
	case "new":
		return runNew(ctx, arguments[1:], output)
	case "edit":
		return runEdit(ctx, arguments[1:])
	case "publish":
		return runPublish(ctx, arguments[1:], input, output)
	case "status":
		return runStatus(arguments[1:], output)
	default:
		return usageError("unknown command %q", arguments[0])
	}
}

// ExitCode maps command errors to stable process exit codes.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 124
	}
	if errors.Is(err, ErrUsage) {
		return 2
	}
	return 1
}

func runNew(ctx context.Context, arguments []string, output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}
	if len(arguments) > 1 {
		return usageError("new accepts at most one name")
	}
	name := ""
	if len(arguments) == 1 {
		name = arguments[0]
	}
	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}
	thought, err := root.Create(name, time.Now())
	if err != nil {
		return err
	}
	if err := metadata.Save(thought.MetadataPath(), metadata.New([]string{"01"})); err != nil {
		return fmt.Errorf("initialize metadata: %w", err)
	}
	if err := editor.Open(ctx, editor.FromEnvironment(), []string{filepath.Join(thought.Path(), "01.md")}); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, thought.Path()); err != nil {
		return fmt.Errorf("write new thought: %w", err)
	}
	return nil
}

func runEdit(ctx context.Context, arguments []string) error {
	if len(arguments) != 1 {
		return usageError("edit requires one thought name or directory")
	}
	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}
	thought, err := root.Thought(arguments[0])
	if err != nil {
		return err
	}
	posts, err := thought.Posts()
	if err != nil {
		return fmt.Errorf("discover posts: %w", err)
	}
	paths := make([]string, 0, len(posts))
	for _, post := range posts {
		paths = append(paths, post.Path)
	}
	return editor.Open(ctx, editor.FromEnvironment(), paths)
}

func runPublish(ctx context.Context, arguments []string, input io.Reader, output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}
	name, targets, err := parsePublishArguments(arguments)
	if err != nil {
		return err
	}
	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}

	var thought archive.Thought
	if name == "" {
		thought, err = root.MostRecentThought()
		if err != nil {
			return fmt.Errorf("find most recent thought: %w", err)
		}
		needsPublication, err := publish.NeedsPublication(thought, targets)
		if err != nil {
			return fmt.Errorf("inspect most recent thought: %w", err)
		}
		if !needsPublication {
			return fmt.Errorf("most recent thought %q is already published; specify a thought to publish", thought.Name())
		}
		confirmed, err := confirmPublish(input, output, thought, targets)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	} else {
		thought, err = root.Thought(name)
		if err != nil {
			return err
		}
	}
	return publish.New(xpost.FromEnvironment()).Publish(ctx, thought, targets, output)
}

func runStatus(arguments []string, output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}
	if len(arguments) != 1 {
		return usageError("status requires one thought name or directory")
	}
	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}
	thought, err := root.Thought(arguments[0])
	if err != nil {
		return err
	}
	posts, err := thought.Posts()
	if err != nil {
		return fmt.Errorf("discover posts: %w", err)
	}
	publication, err := metadata.Load(thought.MetadataPath(), postNames(posts))
	if err != nil {
		return fmt.Errorf("load publication metadata: %w", err)
	}
	if _, err := fmt.Fprintf(output, "thought: %s\n", thought.Name()); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	for _, post := range posts {
		postName := fmt.Sprintf("%02d", post.Number)
		for _, target := range metadata.Targets() {
			record := publication.Get(postName, target)
			if err := writeRecordStatus(output, postName, target, record); err != nil {
				return fmt.Errorf("write status: %w", err)
			}
		}
	}
	return nil
}

func writeRecordStatus(output io.Writer, post, target string, record metadata.Record) error {
	var line strings.Builder
	fmt.Fprintf(&line, "%s %s: %s", post, target, record.Status)
	writeStatusTime(&line, "attempted_at", record.AttemptedAt)
	writeStatusTime(&line, "published_at", record.PublishedAt)
	writeStatusString(&line, "remote_id", record.RemoteID)
	writeStatusString(&line, "remote_cid", record.RemoteCID)
	writeStatusString(&line, "url", record.URL)
	writeStatusString(&line, "parent_id", record.ParentID)
	writeStatusString(&line, "parent_cid", record.ParentCID)
	writeStatusString(&line, "root_id", record.RootID)
	writeStatusString(&line, "root_cid", record.RootCID)
	writeStatusString(&line, "error_kind", record.ErrorKind)
	writeStatusString(&line, "error", record.Error)
	if action := statusAction(record.Status); action != "" {
		writeStatusString(&line, "action", action)
	}
	if _, err := fmt.Fprintln(output, line.String()); err != nil {
		return err
	}
	return nil
}

func writeStatusString(output *strings.Builder, key, value string) {
	if value != "" {
		fmt.Fprintf(output, " %s=%q", key, value)
	}
}

func writeStatusTime(output *strings.Builder, key string, value *time.Time) {
	if value != nil && !value.IsZero() {
		writeStatusString(output, key, value.UTC().Format(time.RFC3339Nano))
	}
}

func statusAction(state metadata.State) string {
	switch state {
	case metadata.StatePublishing:
		return "inspect destination and edit meta.toml to published, failed, or pending"
	case metadata.StateFailed:
		return "run publish to retry"
	case metadata.StateRejected:
		return "fix the post and edit meta.toml status to pending"
	default:
		return ""
	}
}

func parsePublishArguments(arguments []string) (string, []string, error) {
	name := ""
	targets := make([]string, 0)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--target":
			if index+1 >= len(arguments) {
				return "", nil, usageError("--target requires a value")
			}
			index++
			targets = append(targets, arguments[index])
		case strings.HasPrefix(argument, "--target="):
			targets = append(targets, strings.TrimPrefix(argument, "--target="))
		case strings.HasPrefix(argument, "-"):
			return "", nil, usageError("unknown publish option %q", argument)
		case name != "":
			return "", nil, usageError("publish accepts one thought name or directory")
		default:
			name = argument
		}
	}
	return name, targets, nil
}

func confirmPublish(input io.Reader, output io.Writer, thought archive.Thought, targets []string) (bool, error) {
	if input == nil {
		return false, errors.New("input reader is nil")
	}
	if _, err := fmt.Fprintf(output, "publish %q to %s? [y/N] ", thought.Name(), publishTargetText(targets)); err != nil {
		return false, fmt.Errorf("write publication confirmation: %w", err)
	}

	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read publication confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "y" || answer == "yes" {
		return true, nil
	}
	if _, err := fmt.Fprintln(output, "publication cancelled"); err != nil {
		return false, fmt.Errorf("write publication cancellation: %w", err)
	}
	return false, nil
}

func publishTargetText(targets []string) string {
	if len(targets) == 0 {
		return "bluesky and x"
	}
	return strings.Join(targets, ", ")
}

func postNames(posts []archive.Post) []string {
	names := make([]string, 0, len(posts))
	for _, post := range posts {
		names = append(names, fmt.Sprintf("%02d", post.Number))
	}
	return names
}

func requireOutput(output io.Writer) error {
	if output == nil {
		return errors.New("output writer is nil")
	}
	return nil
}

func usageError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s\n\n%s", ErrUsage, fmt.Sprintf(format, arguments...), usageText)
}

func writeUsage(output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}
	if _, err := io.WriteString(output, usageText); err != nil {
		return fmt.Errorf("write usage: %w", err)
	}
	return nil
}
