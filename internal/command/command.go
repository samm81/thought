// Package command parses and executes the thought command-line interface.
package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/samm81/thought/internal/archive"
	"github.com/samm81/thought/internal/editor"
	"github.com/samm81/thought/internal/markdown"
	"github.com/samm81/thought/internal/metadata"
	"github.com/samm81/thought/internal/publish"
	"github.com/samm81/thought/internal/xpost"
)

// ErrUsage indicates that command-line input is invalid.
var ErrUsage = errors.New("usage error")

const usageText = `usage: thought <command> [arguments]

commands:
  new [name]                      create and edit a thought
  edit [name-or-directory]        edit a thought
  publish [name-or-directory] [--target ...]   publish a thought
  status [name-or-directory]      show local publication state
  completion <shell>              print shell completion script

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
	case "__complete":
		return runComplete(arguments[1:], output)
	case "new":
		return runNew(ctx, arguments[1:], output)
	case "edit":
		return runEdit(ctx, arguments[1:])
	case "publish":
		return runPublish(ctx, arguments[1:], input, output)
	case "status":
		return runStatus(arguments[1:], output)
	case "completion":
		return runCompletion(arguments[1:], output)
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

	var name string

	switch len(arguments) {
	case 0:
	case 1:
		name = arguments[0]
	default:
		return usageError("new accepts at most one name")
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

	source, err := thought.ReadSource()
	if err != nil {
		return fmt.Errorf("read new thought source: %w", err)
	}

	if err := editor.Open(ctx, editor.FromEnvironment(), []string{source.Path}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(output, thought.Path()); err != nil {
		return fmt.Errorf("write new thought: %w", err)
	}

	return nil
}

func runEdit(ctx context.Context, arguments []string) error {
	if len(arguments) > 1 {
		return usageError("edit accepts at most one thought name or directory")
	}

	name := strings.Join(arguments, "")

	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}

	thought, err := resolveThought(root, name)
	if err != nil {
		return err
	}

	source, err := thought.ReadSource()
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	return editor.Open(ctx, editor.FromEnvironment(), []string{source.Path})
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

	thought, needsConfirmation, err := resolvePublishThought(root, name, targets)
	if err != nil {
		return err
	}

	if needsConfirmation {
		confirmed, err := confirmPublish(input, output, thought, targets)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	return publish.New(xpost.FromEnvironment()).Publish(ctx, thought, targets, output)
}

func resolvePublishThought(root archive.Root, name string, targets []string) (archive.Thought, bool, error) {
	thought, err := resolveThought(root, name)
	if err != nil {
		if name != "" {
			return thought, false, err
		}

		return archive.Thought{}, false, fmt.Errorf("find most recent thought: %w", err)
	}

	if name != "" {
		return thought, false, nil
	}

	needsPublication, err := publish.NeedsPublication(thought, targets)
	if err != nil {
		return archive.Thought{}, false, fmt.Errorf("inspect most recent thought: %w", err)
	}

	if !needsPublication {
		return archive.Thought{}, false, fmt.Errorf("most recent thought %q is already published; specify a thought to publish", thought.Name())
	}

	return thought, true, nil
}

func runStatus(arguments []string, output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}

	if len(arguments) > 1 {
		return usageError("status accepts at most one thought name or directory")
	}

	name := strings.Join(arguments, "")

	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}

	thought, err := resolveThought(root, name)
	if err != nil {
		return err
	}

	source, err := thought.ReadSource()
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	documents, err := markdown.ParseThread(source.Data, thought.Path())
	if err != nil {
		return fmt.Errorf("parse source: %w", err)
	}

	publication, err := metadata.Load(thought.MetadataPath(), postNames(len(documents)))
	if err != nil {
		return fmt.Errorf("load publication metadata: %w", err)
	}

	if _, err := fmt.Fprintf(output, "thought: %s\n", thought.Name()); err != nil {
		return fmt.Errorf("write status: %w", err)
	}

	for index := range documents {
		postName := fmt.Sprintf("%02d", index+1)
		for _, target := range metadata.Targets() {
			record := publication.Get(postName, target)
			if err := writeRecordStatus(output, postName, target, record); err != nil {
				return fmt.Errorf("write status: %w", err)
			}
		}
	}

	return nil
}

func resolveThought(root archive.Root, name string) (archive.Thought, error) {
	if name == "" {
		return root.MostRecentThought()
	}

	return root.Thought(name)
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
	case metadata.StatePending, metadata.StatePublished:
		return ""
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

func postNames(count int) []string {
	names := make([]string, 0, count)
	for index := range count {
		names = append(names, fmt.Sprintf("%02d", index+1))
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
