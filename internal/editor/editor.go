// Package editor builds safe editor invocations for thought files.
package editor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const defaultCommand = "vi"

// Invocation describes an editor executable and its arguments.
type Invocation struct {
	Name string
	Args []string
}

// FromEnvironment returns VISUAL, EDITOR, or vi.
func FromEnvironment() string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}

	return defaultCommand
}

// Build creates an editor invocation for the supplied files.
func Build(command string, files []string) (Invocation, error) {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return Invocation{}, errors.New("editor command is required")
	}

	if len(files) == 0 {
		return Invocation{}, errors.New("at least one editor file is required")
	}

	args := append([]string(nil), fields[1:]...)
	if isVim(fields[0]) && !contains(args, "-p") {
		insertAt := len(args)
		for index, arg := range args {
			if arg == "--" {
				insertAt = index
				break
			}
		}

		args = append(args, "")
		copy(args[insertAt+1:], args[insertAt:])
		args[insertAt] = "-p"
	}

	args = append(args, files...)

	return Invocation{Name: fields[0], Args: args}, nil
}

// Open launches the configured editor and waits for it to exit.
func Open(ctx context.Context, command string, files []string) error {
	invocation, err := Build(command, files)
	if err != nil {
		return err
	}

	// The editor is an explicit user configuration, so arbitrary executable and arguments are intentional.
	process := exec.CommandContext(ctx, invocation.Name, invocation.Args...) //nolint:gosec // user-configured editor command
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout

	process.Stderr = os.Stderr
	if err := process.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}

		return fmt.Errorf("run editor: %w", err)
	}

	return nil
}

func contains(values []string, wanted string) bool {
	return slices.Contains(values, wanted)
}

func isVim(command string) bool {
	name := strings.ToLower(filepath.Base(command))
	return name == "vim" || name == "nvim" || strings.HasPrefix(name, "vim.") || strings.HasPrefix(name, "nvim.")
}
