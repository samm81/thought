// Package publish sequences independent platform publication attempts.
package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/samm81/thought/internal/archive"
	"github.com/samm81/thought/internal/markdown"
	"github.com/samm81/thought/internal/metadata"
	"github.com/samm81/thought/internal/xpost"
)

// Client publishes one target-specific post.
type Client interface {
	Publish(context.Context, xpost.Request) (xpost.Response, error)
}

// Publisher applies publication state transitions to a local thought.
type Publisher struct {
	client Client
	now    func() time.Time
}

// New creates a publisher using the supplied bridge client.
func New(client Client) Publisher {
	return Publisher{
		client: client,
		now:    time.Now,
	}
}

// Publish publishes a thought to the selected targets.
func (p Publisher) Publish(ctx context.Context, thought archive.Thought, targetNames []string, output io.Writer) error {
	posts, err := thought.Posts()
	if err != nil {
		return fmt.Errorf("discover posts: %w", err)
	}
	documents := make([]markdown.Document, len(posts))
	for index, post := range posts {
		document, err := markdown.ParseFile(post.Path, thought.Path())
		if err != nil {
			return fmt.Errorf("parse post %02d: %w", post.Number, err)
		}
		documents[index] = document
	}
	targets, err := normalizeTargets(targetNames)
	if err != nil {
		return err
	}
	publication, err := metadata.Load(thought.MetadataPath(), postNames(posts))
	if err != nil {
		return fmt.Errorf("load publication metadata: %w", err)
	}
	if output == nil {
		output = io.Discard
	}

	var targetErrors []error
	for _, target := range targets {
		err := p.publishTarget(ctx, thought, posts, documents, target, &publication, output)
		if err == nil {
			continue
		}
		targetErrors = append(targetErrors, err)
		if ctx.Err() != nil {
			break
		}
	}
	if len(targetErrors) > 0 {
		return errors.Join(targetErrors...)
	}
	return nil
}

func (p Publisher) publishTarget(
	ctx context.Context,
	thought archive.Thought,
	posts []archive.Post,
	documents []markdown.Document,
	target string,
	publication *metadata.Document,
	output io.Writer,
) error {
	var parent *xpost.Reference
	var root *xpost.Reference
	for index, post := range posts {
		postName := fmt.Sprintf("%02d", post.Number)
		record := publication.Get(postName, target)
		switch record.Status {
		case metadata.StatePublished:
			current := &xpost.Reference{ID: record.RemoteID, CID: record.RemoteCID}
			if strings.TrimSpace(current.ID) == "" {
				return targetError(target, postName, errors.New("published metadata has no remote id"))
			}
			parent = current
			if root == nil {
				storedRoot := record.RootReply()
				if storedRoot != nil {
					root = &xpost.Reference{ID: storedRoot.ID, CID: storedRoot.CID}
				} else {
					root = current
				}
			}
			continue
		case metadata.StatePublishing:
			return targetError(target, postName, errors.New("publication is still publishing; inspect the destination and edit metadata"))
		case metadata.StateRejected:
			return targetError(target, postName, errors.New("publication was rejected; fix the post and reset metadata to pending"))
		case metadata.StatePending, metadata.StateFailed:
		default:
			return targetError(target, postName, fmt.Errorf("unsupported publication state %q", record.Status))
		}

		if index > 0 && parent == nil {
			return targetError(target, postName, errors.New("parent post is not published"))
		}
		if parent != nil {
			record.ParentID = parent.ID
			record.ParentCID = parent.CID
			if root == nil {
				root = parent
			}
			record.RootID = root.ID
			record.RootCID = root.CID
		}
		if err := record.MarkPublishing(p.now()); err != nil {
			return targetError(target, postName, err)
		}
		if err := publication.Set(postName, target, record); err != nil {
			return targetError(target, postName, err)
		}
		if err := metadata.Save(thought.MetadataPath(), *publication); err != nil {
			return targetError(target, postName, fmt.Errorf("save publishing state: %w", err))
		}

		request := xpost.Request{
			Target: target,
			Text:   documents[index].Text,
		}
		request.Attachments = make([]xpost.Attachment, 0, len(documents[index].Attachments))
		for _, attachment := range documents[index].Attachments {
			request.Attachments = append(request.Attachments, xpost.Attachment{
				Path: attachment.Path,
				Alt:  attachment.Alt,
			})
		}
		if parent != nil {
			request.ReplyTo = &xpost.Reference{ID: parent.ID, CID: parent.CID}
			request.RootReplyTo = &xpost.Reference{ID: root.ID, CID: root.CID}
		}

		response, err := p.client.Publish(ctx, request)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return targetError(target, postName, err)
			}
			if markErr := record.MarkFailed("transport", err.Error()); markErr != nil {
				return targetError(target, postName, markErr)
			}
			if saveErr := publication.Set(postName, target, record); saveErr != nil {
				return targetError(target, postName, saveErr)
			}
			if saveErr := metadata.Save(thought.MetadataPath(), *publication); saveErr != nil {
				return targetError(target, postName, fmt.Errorf("save failed state: %w", saveErr))
			}
			return targetError(target, postName, err)
		}

		switch response.Status {
		case "failed":
			errorKind, errorMessage := responseError(response, "transport")
			if markErr := record.MarkFailed(errorKind, errorMessage); markErr != nil {
				return targetError(target, postName, markErr)
			}
			if err := publication.Set(postName, target, record); err != nil {
				return targetError(target, postName, err)
			}
			if err := metadata.Save(thought.MetadataPath(), *publication); err != nil {
				return targetError(target, postName, fmt.Errorf("save failed state: %w", err))
			}
			return targetError(target, postName, errors.New(errorMessage))
		case "rejected":
			errorKind, errorMessage := responseError(response, "validation")
			if markErr := record.MarkRejected(errorKind, errorMessage); markErr != nil {
				return targetError(target, postName, markErr)
			}
			if err := publication.Set(postName, target, record); err != nil {
				return targetError(target, postName, err)
			}
			if err := metadata.Save(thought.MetadataPath(), *publication); err != nil {
				return targetError(target, postName, fmt.Errorf("save rejected state: %w", err))
			}
			return targetError(target, postName, errors.New(errorMessage))
		case "published":
			result := metadata.Reference{ID: response.RemoteID, CID: response.RemoteCID}
			if err := record.MarkPublished(p.now(), result, response.URL); err != nil {
				return targetError(target, postName, err)
			}
			if err := publication.Set(postName, target, record); err != nil {
				return targetError(target, postName, err)
			}
			if err := metadata.Save(thought.MetadataPath(), *publication); err != nil {
				return targetError(target, postName, fmt.Errorf("save published state: %w", err))
			}
			current := &xpost.Reference{ID: response.RemoteID, CID: response.RemoteCID}
			parent = current
			if root == nil {
				root = current
			}
			if _, err := fmt.Fprintf(output, "%s %s: published\n", target, postName); err != nil {
				return targetError(target, postName, fmt.Errorf("write publication status: %w", err))
			}
		default:
			return targetError(target, postName, fmt.Errorf("unsupported xpost response status %q", response.Status))
		}
	}
	return nil
}

func normalizeTargets(values []string) ([]string, error) {
	if len(values) == 0 {
		return metadata.Targets(), nil
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		target := strings.ToLower(strings.TrimSpace(value))
		if !validTarget(target) {
			return nil, fmt.Errorf("unsupported target %q", value)
		}
		seen[target] = true
	}
	targets := make([]string, 0, len(seen))
	for target := range seen {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets, nil
}

func validTarget(target string) bool {
	return target == metadata.TargetBluesky || target == metadata.TargetX
}

func postNames(posts []archive.Post) []string {
	names := make([]string, 0, len(posts))
	for _, post := range posts {
		names = append(names, fmt.Sprintf("%02d", post.Number))
	}
	return names
}

func targetError(target, post string, err error) error {
	return fmt.Errorf("%s %s: %w", target, post, err)
}

func responseError(response xpost.Response, defaultKind string) (string, string) {
	kind := strings.TrimSpace(response.ErrorKind)
	if kind == "" {
		kind = defaultKind
	}
	message := strings.TrimSpace(response.Error)
	if message == "" {
		message = fmt.Sprintf("xpost bridge returned %s without an error", response.Status)
	}
	return kind, message
}
