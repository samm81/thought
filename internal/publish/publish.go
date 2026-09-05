// Package publish sequences independent platform publication attempts.
package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	_, documents, err := readDocuments(thought)
	if err != nil {
		return false, err
	}

	targets, err := normalizeTargets(targetNames)
	if err != nil {
		return false, err
	}

	publication, err := metadata.Load(thought.MetadataPath(), postNames(len(documents)))
	if err != nil {
		return false, fmt.Errorf("load publication metadata: %w", err)
	}

	for index := range documents {
		postName := postName(index)
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
	source, documents, err := readDocuments(thought)
	if err != nil {
		return err
	}

	targets, err := normalizeTargets(targetNames)
	if err != nil {
		return err
	}

	publication, err := metadata.Load(thought.MetadataPath(), postNames(len(documents)))
	if err != nil {
		return fmt.Errorf("load publication metadata: %w", err)
	}

	sourceHash := hashSource(source.Data)
	if publication.SourceHash != "" && publication.SourceHash != sourceHash {
		return sourceChangedError(publication.SourceHash, sourceHash)
	}

	if output == nil {
		output = io.Discard
	}

	preflightErrors, err := p.validatePosts(ctx, documents, targets, &publication)
	if err != nil {
		return err
	}

	if err := lockSourceIfNeeded(thought, sourceHash, documents, targets, preflightErrors, &publication); err != nil {
		return err
	}

	targetErrors := p.publishTargets(ctx, thought, documents, sourceHash, targets, preflightErrors, &publication, output)

	if len(targetErrors) > 0 {
		return errors.Join(targetErrors...)
	}

	return nil
}

func readDocuments(thought archive.Thought) (archive.Source, []markdown.Document, error) {
	source, err := thought.ReadSource()
	if err != nil {
		return archive.Source{}, nil, fmt.Errorf("read source: %w", err)
	}

	documents, err := markdown.ParseThread(source.Data, thought.Path())
	if err != nil {
		return archive.Source{}, nil, fmt.Errorf("parse source: %w", err)
	}

	return source, documents, nil
}

func (p Publisher) publishTargets(
	ctx context.Context,
	thought archive.Thought,
	documents []markdown.Document,
	sourceHash string,
	targets []string,
	preflightErrors map[string]error,
	publication *metadata.Document,
	output io.Writer,
) []error {
	var targetErrors []error

	for _, target := range targets {
		if err := preflightErrors[target]; err != nil {
			targetErrors = append(targetErrors, err)
			continue
		}

		err := p.publishTarget(ctx, thought, documents, sourceHash, target, publication, output)
		if err == nil {
			continue
		}

		targetErrors = append(targetErrors, err)

		if ctx.Err() != nil {
			break
		}
	}

	return targetErrors
}

func (p Publisher) validatePosts(
	ctx context.Context,
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

		for index := range documents {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			postName := postName(index)
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
	documents []markdown.Document,
	sourceHash string,
	target string,
	publication *metadata.Document,
	output io.Writer,
) error {
	var (
		parent *xpost.Reference
		root   *xpost.Reference
	)

	for index := range documents {
		postName := postName(index)

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

		if err := verifySourceHash(thought, sourceHash); err != nil {
			return targetError(target, postName, err)
		}

		if err := p.startPublication(thought, target, postName, publication, &record); err != nil {
			return targetError(target, postName, err)
		}

		current, err := p.publishPost(ctx, thought, documents[index], sourceHash, target, postName, record, publication, output)
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
	sourceHash string,
	target, postName string,
	record metadata.Record,
	publication *metadata.Document,
	output io.Writer,
) (*xpost.Reference, error) {
	request := buildRequest(document, target, record)
	if err := verifySourceHash(thought, sourceHash); err != nil {
		return nil, targetError(target, postName, err)
	}

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

func postNames(count int) []string {
	names := make([]string, 0, count)
	for index := range count {
		names = append(names, postName(index))
	}

	return names
}

func postName(index int) string {
	return fmt.Sprintf("%02d", index+1)
}

func needsPublication(documents []markdown.Document, target string, publication metadata.Document) bool {
	for index := range documents {
		if publication.Get(postName(index), target).Status != metadata.StatePublished {
			return true
		}
	}

	return false
}

func lockSourceIfNeeded(
	thought archive.Thought,
	sourceHash string,
	documents []markdown.Document,
	targets []string,
	preflightErrors map[string]error,
	publication *metadata.Document,
) error {
	for _, target := range targets {
		if preflightErrors[target] != nil || !needsPublication(documents, target, *publication) {
			continue
		}

		if err := verifySourceHash(thought, sourceHash); err != nil {
			return err
		}

		return lockSource(thought, sourceHash, publication)
	}

	return nil
}

func lockSource(thought archive.Thought, sourceHash string, publication *metadata.Document) error {
	if publication.SourceHash != "" {
		return nil
	}

	if hasPublicationActivity(*publication) {
		return errors.New("publication metadata has no source hash; inspect existing publication and set source_hash before retrying")
	}

	if err := publication.SetSourceHash(sourceHash); err != nil {
		return err
	}

	if err := metadata.Save(thought.MetadataPath(), *publication); err != nil {
		return fmt.Errorf("save source hash: %w", err)
	}

	return nil
}

func hasPublicationActivity(publication metadata.Document) bool {
	for _, targets := range publication.Posts {
		for _, record := range targets {
			if record.Status != metadata.StatePending {
				return true
			}
		}
	}

	return false
}

func verifySourceHash(thought archive.Thought, expected string) error {
	source, err := thought.ReadSource()
	if err != nil {
		return fmt.Errorf("verify source: %w", err)
	}

	actual := hashSource(source.Data)
	if actual != expected {
		return sourceChangedError(expected, actual)
	}

	return nil
}

func hashSource(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func sourceChangedError(expected, actual string) error {
	return fmt.Errorf("source file changed after publication began (expected %s, found %s); inspect destinations and recover metadata manually", expected, actual)
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
