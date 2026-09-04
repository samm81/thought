// Package markdown parses authored Markdown and renders platform text.
package markdown

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Attachment is one image declared by a Markdown post.
type Attachment struct {
	Path string
	Alt  string
}

// Document is the platform-ready content of one Markdown post.
type Document struct {
	Text        string
	Attachments []Attachment
}

// Parse parses Markdown text and extracts a trailing image block.
func Parse(data []byte) (Document, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	textLines := make([]string, 0, len(lines))
	attachments := make([]Attachment, 0)
	inFence := false
	seenAttachment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFence(trimmed) {
			inFence = !inFence

			textLines = append(textLines, line)

			continue
		}

		if !inFence {
			attachment, ok, err := parseImageLine(trimmed)
			if err != nil {
				return Document{}, err
			}

			if ok {
				seenAttachment = true

				attachments = append(attachments, attachment)

				continue
			}

			if strings.Contains(line, "![") && strings.Contains(line, "](") {
				return Document{}, errors.New("image declarations must be complete trailing lines")
			}
		}

		if seenAttachment && trimmed != "" {
			return Document{}, errors.New("image declarations must form a trailing attachment block")
		}

		if !seenAttachment {
			textLines = append(textLines, line)
		}
	}

	return Document{
		Text:        renderText(textLines),
		Attachments: attachments,
	}, nil
}

// ParseFile parses a post and resolves its attachments inside thoughtPath.
func ParseFile(path, thoughtPath string) (Document, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from the validated local thought archive
	if err != nil {
		return Document{}, fmt.Errorf("read markdown: %w", err)
	}

	document, err := Parse(data)
	if err != nil {
		return Document{}, fmt.Errorf("parse markdown: %w", err)
	}

	for index, attachment := range document.Attachments {
		resolved, err := ResolveAttachment(thoughtPath, attachment.Path)
		if err != nil {
			return Document{}, fmt.Errorf("attachment %q: %w", attachment.Path, err)
		}

		document.Attachments[index].Path = resolved
	}

	return document, nil
}

// ResolveAttachment validates and resolves one local attachment path.
func ResolveAttachment(thoughtPath, relativePath string) (string, error) {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return "", errors.New("attachment path is empty")
	}

	if !validAttachmentPath(relativePath) {
		return "", errors.New("attachment path must be local and relative")
	}

	if filepath.IsAbs(relativePath) || !filepath.IsLocal(relativePath) {
		return "", errors.New("attachment path must stay inside the thought directory")
	}

	root, err := filepath.Abs(filepath.Clean(thoughtPath))
	if err != nil {
		return "", fmt.Errorf("resolve thought directory: %w", err)
	}

	candidate := filepath.Join(root, filepath.Clean(relativePath))

	if !pathWithin(root, candidate) {
		return "", errors.New("attachment path must stay inside the thought directory")
	}

	info, err := os.Stat(candidate)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("attachment file does not exist")
		}

		return "", fmt.Errorf("inspect attachment: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", errors.New("attachment is not a regular file")
	}

	if err := validateAttachmentLink(root, candidate); err != nil {
		return "", err
	}

	return candidate, nil
}

func validAttachmentPath(path string) bool {
	return !strings.HasPrefix(path, "~") && !strings.ContainsRune(path, 0) && !strings.Contains(path, `\`)
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false
	}

	return true
}

func validateAttachmentLink(root, candidate string) error {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve thought directory: %w", err)
	}

	candidateReal, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fmt.Errorf("resolve attachment: %w", err)
	}

	if !pathWithin(rootReal, candidateReal) {
		return errors.New("attachment symlink escapes the thought directory")
	}

	return nil
}

func parseImageLine(line string) (Attachment, bool, error) {
	if !strings.HasPrefix(line, "![") {
		return Attachment{}, false, nil
	}

	altEnd := strings.Index(line[2:], "](")
	if altEnd < 0 || !strings.HasSuffix(line, ")") {
		return Attachment{}, false, errors.New("invalid image declaration")
	}

	altEnd += 2

	path := strings.TrimSpace(line[altEnd+2 : len(line)-1])
	if path == "" {
		return Attachment{}, false, errors.New("image declaration has no path")
	}

	if strings.HasPrefix(path, "<") && strings.HasSuffix(path, ">") {
		path = strings.TrimSuffix(strings.TrimPrefix(path, "<"), ">")
	}

	if strings.Contains(path, "\"") || strings.Contains(path, "'") {
		return Attachment{}, false, errors.New("image titles are not supported")
	}

	return Attachment{
		Alt:  line[2:altEnd],
		Path: path,
	}, true, nil
}

func renderText(lines []string) string {
	parts := make([]string, 0, len(lines))
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFence(trimmed) {
			inFence = !inFence
			continue
		}

		if inFence {
			parts = append(parts, strings.TrimRight(line, " \t"))
			continue
		}

		parts = append(parts, renderLine(line))
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func renderLine(line string) string {
	line = strings.TrimRight(line, " \t")

	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "#") {
		count := 0
		for count < len(trimmed) && trimmed[count] == '#' {
			count++
		}

		if count <= 6 && count < len(trimmed) && trimmed[count] == ' ' {
			line = trimmed[count+1:]
		}
	}

	line = replaceLinks(line)
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "~~", "")

	return line
}

func replaceLinks(line string) string {
	var output strings.Builder

	for {
		start := strings.Index(line, "[")
		if start < 0 {
			output.WriteString(line)
			return output.String()
		}

		closeLabel := strings.Index(line[start+1:], "](")
		if closeLabel < 0 {
			output.WriteString(line)
			return output.String()
		}

		closeLabel += start + 1

		closeURL := strings.IndexByte(line[closeLabel+2:], ')')
		if closeURL < 0 {
			output.WriteString(line)
			return output.String()
		}

		closeURL += closeLabel + 2
		label := line[start+1 : closeLabel]
		url := line[closeLabel+2 : closeURL]
		output.WriteString(line[:start])

		switch label {
		case "":
			output.WriteString(url)
		case url:
			output.WriteString(label)
		default:
			fmt.Fprintf(&output, "%s (%s)", label, url)
		}

		line = line[closeURL+1:]
	}
}

func isFence(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}
