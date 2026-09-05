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
	Validate(context.Context, xpost.Request) error
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

// NeedsPublication reports whether any selected post still needs publication.
func NeedsPublication(thought archive.Thought, targetNames []string) (bool, error) {
	posts, err := thought.Posts()
	if err != nil {
		return false, fmt.Errorf("discover posts: %w", err)
	}

	targets, err := normalizeTargets(targetNames)
	if err != nil {
		return false, err
	}

	publication, err := metadata.Load(thought.MetadataPath(), postNames(posts))
	if err != nil {
		return false, fmt.Errorf("load publication metadata: %w", err)
	}

	for _, post := range posts {
		postName := fmt.Sprintf("%02d", post.Number)
		for _, target := range targets {
			if publication.Get(postName, target).Status != metadata.StatePublished {
				return true, nil
			}
		}
	}

	return false, nil
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

	preflightErrors, err := p.validatePosts(ctx, posts, documents, targets, &publication)
	if err != nil {
		return err
	}

	var targetErrors []error

	for _, target := range targets {
		if err := preflightErrors[target]; err != nil {
			targetErrors = append(targetErrors, err)
			continue
		}

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

func (p Publisher) validatePosts(
	ctx context.Context,
	posts []archive.Post,
	documents []markdown.Document,
	targets []string,
	publication *metadata.Document,
) (map[string]error, error) {
	validationErrors := make(map[string][]error, len(targets))

	for _, target := range targets {
		var (
			parent *xpost.Reference
			root   *xpost.Reference
		)

		for index, post := range posts {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			postName := fmt.Sprintf("%02d", post.Number)
			record := publication.Get(postName, target)

			published, err := advancePublishedRecord(record, &parent, &root)
			if err != nil {
				validationErrors[target] = append(validationErrors[target], targetError(target, postName, err))
				continue
			}

			if published {
				continue
			}

			record = recordForValidation(record, parent, &root)

			request := buildRequest(documents[index], target, record)
			if err := p.client.Validate(ctx, request); err != nil {
				if contextErr := ctx.Err(); contextErr != nil {
					return nil, contextErr
				}

				validationErrors[target] = append(validationErrors[target], targetError(target, postName, err))
			}
		}
	}

	joinedErrors := make(map[string]error, len(validationErrors))
	for target, targetValidationErrors := range validationErrors {
		joinedErrors[target] = errors.Join(targetValidationErrors...)
	}

	return joinedErrors, nil
}

func recordForValidation(record metadata.Record, parent *xpost.Reference, root **xpost.Reference) metadata.Record {
	if parent == nil {
		record.ParentID = ""
		record.ParentCID = ""
		record.RootID = ""
		record.RootCID = ""

		return record
	}

	record.ParentID = parent.ID
	record.ParentCID = parent.CID

	if *root == nil {
		*root = parent
	}

	record.RootID = (*root).ID
	record.RootCID = (*root).CID

	return record
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
	var (
		parent *xpost.Reference
		root   *xpost.Reference
	)

	for index, post := range posts {
		postName := fmt.Sprintf("%02d", post.Number)

		record := publication.Get(postName, target)

		published, err := advancePublishedRecord(record, &parent, &root)
		if err != nil {
			return targetError(target, postName, err)
		}

		if published {
			continue
		}

		if err := prepareReply(&record, index, parent, &root); err != nil {
			return targetError(target, postName, err)
		}

		if err := p.startPublication(thought, target, postName, publication, &record); err != nil {
			return targetError(target, postName, err)
		}

		current, err := p.publishPost(ctx, thought, documents[index], target, postName, record, publication, output)
		if err != nil {
			return err
		}

		parent = current
		if root == nil {
			root = current
		}
	}

	return nil
}

func advancePublishedRecord(record metadata.Record, parent, root **xpost.Reference) (bool, error) {
	switch record.Status {
	case metadata.StatePublished:
		current := &xpost.Reference{ID: record.RemoteID, CID: record.RemoteCID}
		if strings.TrimSpace(current.ID) == "" {
			return false, errors.New("published metadata has no remote id")
		}

		*parent = current

		if *root == nil {
			storedRoot := record.RootReply()
			if storedRoot != nil {
				*root = &xpost.Reference{ID: storedRoot.ID, CID: storedRoot.CID}
			} else {
				*root = current
			}
		}

		return true, nil
	case metadata.StatePublishing:
		return false, errors.New("publication is still publishing; inspect the destination and edit metadata")
	case metadata.StateRejected:
		return false, errors.New("publication was rejected; fix the post and reset metadata to pending")
	case metadata.StatePending, metadata.StateFailed:
		return false, nil
	default:
		return false, fmt.Errorf("unsupported publication state %q", record.Status)
	}
}

func prepareReply(record *metadata.Record, index int, parent *xpost.Reference, root **xpost.Reference) error {
	if index > 0 && parent == nil {
		return errors.New("parent post is not published")
	}

	if parent == nil {
		return nil
	}

	record.ParentID = parent.ID

	record.ParentCID = parent.CID
	if *root == nil {
		*root = parent
	}

	record.RootID = (*root).ID
	record.RootCID = (*root).CID

	return nil
}

func (p Publisher) startPublication(
	thought archive.Thought,
	target, postName string,
	publication *metadata.Document,
	record *metadata.Record,
) error {
	if err := record.MarkPublishing(p.now()); err != nil {
		return err
	}

	if err := savePublicationState(thought, target, postName, publication, *record, "save publishing state"); err != nil {
		return err
	}

	return nil
}

func (p Publisher) publishPost(
	ctx context.Context,
	thought archive.Thought,
	document markdown.Document,
	target, postName string,
	record metadata.Record,
	publication *metadata.Document,
	output io.Writer,
) (*xpost.Reference, error) {
	request := buildRequest(document, target, record)

	response, err := p.client.Publish(ctx, request)
	if err != nil {
		return nil, p.failTransport(thought, target, postName, publication, record, err)
	}

	switch response.Status {
	case "failed":
		return nil, p.failResponse(thought, target, postName, publication, record, response, false, "transport", "save failed state")
	case "rejected":
		return nil, p.failResponse(thought, target, postName, publication, record, response, true, "validation", "save rejected state")
	case "published":
		return p.completePublication(thought, target, postName, publication, record, response, output)
	default:
		return nil, targetError(target, postName, fmt.Errorf("unsupported xpost response status %q", response.Status))
	}
}

func (p Publisher) failTransport(
	thought archive.Thought,
	target, postName string,
	publication *metadata.Document,
	record metadata.Record,
	transportErr error,
) error {
	if markErr := record.MarkFailed("transport", transportErr.Error()); markErr != nil {
		return targetError(target, postName, markErr)
	}

	if saveErr := savePublicationState(thought, target, postName, publication, record, "save failed state"); saveErr != nil {
		return targetError(target, postName, saveErr)
	}

	return targetError(target, postName, transportErr)
}

func (p Publisher) failResponse(
	thought archive.Thought,
	target, postName string,
	publication *metadata.Document,
	record metadata.Record,
	response xpost.Response,
	rejected bool,
	defaultKind, saveContext string,
) error {
	errorKind, errorMessage := responseError(response, defaultKind)

	if rejected {
		if markErr := record.MarkRejected(errorKind, errorMessage); markErr != nil {
			return targetError(target, postName, markErr)
		}
	} else if markErr := record.MarkFailed(errorKind, errorMessage); markErr != nil {
		return targetError(target, postName, markErr)
	}

	if saveErr := savePublicationState(thought, target, postName, publication, record, saveContext); saveErr != nil {
		return targetError(target, postName, saveErr)
	}

	return targetError(target, postName, errors.New(errorMessage))
}

func (p Publisher) completePublication(
	thought archive.Thought,
	target, postName string,
	publication *metadata.Document,
	record metadata.Record,
	response xpost.Response,
	output io.Writer,
) (*xpost.Reference, error) {
	result := metadata.Reference{ID: response.RemoteID, CID: response.RemoteCID}
	if err := record.MarkPublished(p.now(), result, response.URL); err != nil {
		return nil, targetError(target, postName, err)
	}

	if saveErr := savePublicationState(thought, target, postName, publication, record, "save published state"); saveErr != nil {
		return nil, targetError(target, postName, saveErr)
	}

	if _, err := fmt.Fprintf(output, "%s %s: published\n", target, postName); err != nil {
		return nil, targetError(target, postName, fmt.Errorf("write publication status: %w", err))
	}

	return &xpost.Reference{ID: response.RemoteID, CID: response.RemoteCID}, nil
}

func buildRequest(document markdown.Document, target string, record metadata.Record) xpost.Request {
	request := xpost.Request{
		Target:      target,
		Text:        document.Text,
		Attachments: make([]xpost.Attachment, 0, len(document.Attachments)),
	}
	for _, attachment := range document.Attachments {
		request.Attachments = append(request.Attachments, xpost.Attachment{
			Path: attachment.Path,
			Alt:  attachment.Alt,
		})
	}

	if record.ParentID != "" {
		request.ReplyTo = &xpost.Reference{ID: record.ParentID, CID: record.ParentCID}
		request.RootReplyTo = &xpost.Reference{ID: record.RootID, CID: record.RootCID}
	}

	return request
}

func savePublicationState(
	thought archive.Thought,
	target, postName string,
	publication *metadata.Document,
	record metadata.Record,
	saveContext string,
) error {
	if err := publication.Set(postName, target, record); err != nil {
		return err
	}

	if err := metadata.Save(thought.MetadataPath(), *publication); err != nil {
		return fmt.Errorf("%s: %w", saveContext, err)
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
