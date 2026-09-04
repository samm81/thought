// Package main provides the thought command-line executable.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/samm81/thought/internal/command"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exitCode := 0

	if err := command.Run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "thought:", err)
		exitCode = command.ExitCode(err)
	}

	stop()

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
