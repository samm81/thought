package command

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		arguments     []string
		wantOutput    string
		wantError     error
		wantErrorText string
	}{
		{
			name:       "no arguments",
			wantOutput: usageText,
		},
		{
			name:       "long help",
			arguments:  []string{"--help"},
			wantOutput: usageText,
		},
		{
			name:       "short help",
			arguments:  []string{"-h"},
			wantOutput: usageText,
		},
		{
			name:      "planned command",
			arguments: []string{"new"},
			wantError: ErrNotImplemented,
		},
		{
			name:          "unknown command",
			arguments:     []string{"unknown"},
			wantError:     ErrUsage,
			wantErrorText: "usage:",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			err := Run(t.Context(), test.arguments, &output)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Run() error = %v, want %v", err, test.wantError)
			}
			if test.wantErrorText != "" && (err == nil || !strings.Contains(err.Error(), test.wantErrorText)) {
				t.Fatalf("Run() error = %q, want it to contain %q", err, test.wantErrorText)
			}

			if test.wantOutput == usageText && output.String() != test.wantOutput {
				t.Fatalf("Run() output = %q, want %q", output.String(), test.wantOutput)
			}
			if test.wantOutput != usageText && !strings.Contains(output.String(), test.wantOutput) {
				t.Fatalf("Run() output = %q, want it to contain %q", output.String(), test.wantOutput)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", want: 0},
		{name: "usage", err: ErrUsage, want: 2},
		{name: "not implemented", err: ErrNotImplemented, want: 1},
		{name: "canceled", err: context.Canceled, want: 130},
		{name: "deadline", err: context.DeadlineExceeded, want: 124},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := ExitCode(test.err); got != test.want {
				t.Fatalf("ExitCode(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}
