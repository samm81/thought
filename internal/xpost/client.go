// Package xpost communicates with the external xpost subprocess bridge.
package xpost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 2 * time.Minute

// Attachment identifies one image to send to a target.
type Attachment struct {
	Path string `json:"path"`
	Alt  string `json:"alt,omitempty"`
}

// Reference identifies a previously published post.
type Reference struct {
	ID  string `json:"id"`
	CID string `json:"cid,omitempty"`
}

// Request is one target-specific publication request.
type Request struct {
	Target      string
	Text        string
	Attachments []Attachment
	ReplyTo     *Reference
	RootReplyTo *Reference
}

// Response is the result returned by xpost bridge.
type Response struct {
	Status    string `json:"status"`
	RemoteID  string `json:"remote_id,omitempty"`
	RemoteCID string `json:"remote_cid,omitempty"`
	URL       string `json:"url,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorKind string `json:"error_kind,omitempty"`
}

// Client invokes the xpost subprocess.
type Client struct {
	command string
	timeout time.Duration
}

// FromEnvironment creates a client using THOUGHT_XPOST or xpost on PATH.
func FromEnvironment() Client {
	return New(os.Getenv("THOUGHT_XPOST"), defaultTimeout)
}

// New creates a client with an explicit executable and timeout.
func New(command string, timeout time.Duration) Client {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "xpost"
	}

	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return Client{command: command, timeout: timeout}
}

// Publish sends one request to the xpost bridge.
func (c Client) Publish(ctx context.Context, request Request) (Response, error) {
	payload, err := json.Marshal(struct {
		Operation   string       `json:"operation"`
		Target      string       `json:"target"`
		Text        string       `json:"text"`
		Attachments []Attachment `json:"attachments,omitempty"`
		ReplyTo     *Reference   `json:"reply_to,omitempty"`
		RootReplyTo *Reference   `json:"root_reply_to,omitempty"`
	}{
		Operation:   "publish",
		Target:      request.Target,
		Text:        request.Text,
		Attachments: request.Attachments,
		ReplyTo:     request.ReplyTo,
		RootReplyTo: request.RootReplyTo,
	})
	if err != nil {
		return Response{}, fmt.Errorf("encode xpost request: %w", err)
	}

	commandContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// The bridge executable is an explicit user configuration.
	process := exec.CommandContext(commandContext, c.command, "bridge") //nolint:gosec // user-configured bridge command
	process.Stdin = bytes.NewReader(payload)

	var (
		standardOutput bytes.Buffer
		standardError  bytes.Buffer
	)

	process.Stdout = &standardOutput

	process.Stderr = &standardError
	if err := process.Run(); err != nil {
		if commandContext.Err() != nil {
			return Response{}, fmt.Errorf("run xpost bridge: %w", commandContext.Err())
		}

		return Response{}, ProcessError{
			Command: c.command,
			Stderr:  strings.TrimSpace(standardError.String()),
			Err:     err,
		}
	}

	response, err := decodeResponse(standardOutput.Bytes())
	if err != nil {
		return Response{}, err
	}

	switch response.Status {
	case "published":
		if strings.TrimSpace(response.RemoteID) == "" {
			return Response{}, errors.New("xpost bridge returned published without remote id")
		}
	case "failed", "rejected":
		if strings.TrimSpace(response.Error) == "" {
			response.Error = "xpost bridge returned an empty error"
		}
	default:
		return Response{}, fmt.Errorf("xpost bridge returned unsupported status %q", response.Status)
	}

	return response, nil
}

// ProcessError describes a failed xpost process invocation.
type ProcessError struct {
	Command string
	Stderr  string
	Err     error
}

func (e ProcessError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("run %s: %v", e.Command, e.Err)
	}

	return fmt.Sprintf("run %s: %v: %s", e.Command, e.Err, e.Stderr)
}

// Unwrap exposes the process error for errors.Is and errors.As.
func (e ProcessError) Unwrap() error {
	return e.Err
}

func decodeResponse(data []byte) (Response, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var response Response
	if err := decoder.Decode(&response); err != nil {
		return Response{}, fmt.Errorf("decode xpost response: %w", err)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Response{}, errors.New("decode xpost response: multiple JSON values")
		}

		return Response{}, fmt.Errorf("decode xpost response: %w", err)
	}

	return response, nil
}
