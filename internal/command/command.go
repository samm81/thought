// Package command parses and executes the thought command-line interface.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
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
  edit <name>                     edit a thought
  publish <name> [--target ...]   publish a thought
  status <name>                   show local publication state

options:
  -h, --help                     show this help
`

// Run parses arguments and executes one command.
func Run(ctx context.Context, arguments []string, output io.Writer) error {
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
		return runPublish(ctx, arguments[1:], output)
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
		return usageError("edit requires one thought name")
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

func runPublish(ctx context.Context, arguments []string, output io.Writer) error {
	name, targets, err := parsePublishArguments(arguments)
	if err != nil {
		return err
	}
	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}
	thought, err := root.Thought(name)
	if err != nil {
		return err
	}
	return publish.New(xpost.FromEnvironment()).Publish(ctx, thought, targets, output)
}

func runStatus(arguments []string, output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}
	if len(arguments) != 1 {
		return usageError("status requires one thought name")
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
			if _, err := fmt.Fprintf(output, "%s %s: %s", postName, target, record.Status); err != nil {
				return fmt.Errorf("write status: %w", err)
			}
			if record.URL != "" {
				if _, err := fmt.Fprintf(output, " %s", record.URL); err != nil {
					return fmt.Errorf("write status: %w", err)
				}
			}
			if record.Error != "" {
				if _, err := fmt.Fprintf(output, " (%s)", record.Error); err != nil {
					return fmt.Errorf("write status: %w", err)
				}
			}
			if _, err := fmt.Fprintln(output); err != nil {
				return fmt.Errorf("write status: %w", err)
			}
		}
	}
	return nil
}

func parsePublishArguments(arguments []string) (string, []string, error) {
	if len(arguments) == 0 || strings.TrimSpace(arguments[0]) == "" || strings.HasPrefix(arguments[0], "-") {
		return "", nil, usageError("publish requires one thought name")
	}
	name := arguments[0]
	targets := make([]string, 0)
	for index := 1; index < len(arguments); index++ {
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
		default:
			return "", nil, usageError("unknown publish option %q", argument)
		}
	}
	return name, targets, nil
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
