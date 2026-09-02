// Package command parses the thought command-line interface.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrUsage indicates that command-line input is invalid.
	ErrUsage = errors.New("usage error")

	// ErrNotImplemented indicates that a command is part of the planned interface but is not wired yet.
	ErrNotImplemented = errors.New("not implemented")
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

// Run parses arguments and executes the command-line surface.
//
// The command implementations are intentionally left as the next work slice;
// keeping parsing and process wiring in place makes that work independently testable.
func Run(ctx context.Context, arguments []string, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(arguments) == 0 || arguments[0] == "-h" || arguments[0] == "--help" || arguments[0] == "help" {
		return writeUsage(output)
	}

	switch arguments[0] {
	case "new", "edit", "publish", "status":
		return fmt.Errorf("%w: %s", ErrNotImplemented, arguments[0])
	default:
		return fmt.Errorf("%w: unknown command %q\n\n%s", ErrUsage, arguments[0], usageText)
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

func writeUsage(output io.Writer) error {
	if output == nil {
		return errors.New("output writer is nil")
	}

	_, err := io.WriteString(output, usageText)
	return err
}
