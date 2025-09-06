package model

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/go-git/go-git/v5/utils/diff"
	"github.com/pterm/pterm"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffMergeService defines the interface for diff and merge operations
type DiffMergeService interface {
	ConvertToEncodedObject(content string) (plumbing.EncodedObject, error)
	FindDiff(localContent, remoteContent string) (plumbing.EncodedObject, error)
	GetChanges(obj plumbing.EncodedObject, diff plumbing.EncodedObject) ([]diffmatchpatch.Diff, error)
	ApplyDiff(baseContent string, changes []diffmatchpatch.Diff) ([]byte, error)
	PrettyPrint(diffs []diffmatchpatch.Diff) string
}

// diffMergeService implements the DiffMergeService interface
type diffMergeService struct{}

// NewDiffMergeService creates a new instance of DiffMergeService
func NewDiffMergeService() DiffMergeService {
	return &diffMergeService{}
}
func (s *diffMergeService) ConvertToEncodedObject(content string) (plumbing.EncodedObject, error) {
	odb := memory.NewStorage()

	// Create blob for local content
	localOid := odb.NewEncodedObject()
	localOid.SetType(plumbing.BlobObject)
	localOid.SetSize(int64(len(content)))
	writer, err := localOid.Writer()
	if err != nil {
		return nil, err
	}
	writer.Write([]byte(content))
	writer.Close()
	return localOid, err

}

// FindDiffAndMergeWithGitObjects uses go-git's merge functionality with git objects
func (s *diffMergeService) FindDiff(localContent, remoteContent string) (plumbing.EncodedObject, error) {
	localOid, err := s.ConvertToEncodedObject(localContent)
	if err != nil {
		return nil, err
	}
	remoteOid, err := s.ConvertToEncodedObject(remoteContent)
	if err != nil {
		return nil, err
	}

	delta, err := packfile.GetDelta(localOid, remoteOid)
	return delta, err
}

// GetChanges extracts the diff changes between base object and diff object
func (s *diffMergeService) GetChanges(obj plumbing.EncodedObject, diffs plumbing.EncodedObject) ([]diffmatchpatch.Diff, error) {
	// Read the base object content
	baseReader, err := obj.Reader()
	if err != nil {
		return nil, err
	}
	defer baseReader.Close()

	baseContent, err := io.ReadAll(baseReader)
	if err != nil {
		return nil, err
	}

	// Read the target content from the diff object
	// The diff object contains the full target content after applying packfile.GetDelta
	deltaReader, err := diffs.Reader()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer deltaReader.Close()

	deltaContent, err := io.ReadAll(deltaReader)
	if err != nil {
		return nil, err
	}

	if len(deltaContent) == 0 {
		return []diffmatchpatch.Diff{}, nil
	}

	if len(baseContent) == 0 {
		// If base is empty, treat everything as an insert
		return []diffmatchpatch.Diff{
			{Type: diffmatchpatch.DiffInsert, Text: string(bytes.Trim(deltaContent, "\x00"))},
		}, nil
	}

	// First try to apply as a delta
	targetContent, err := packfile.PatchDelta(baseContent, deltaContent)
	if err != nil {
		if errors.Is(err, packfile.ErrInvalidDelta) {
			return []diffmatchpatch.Diff{}, nil
		}
		// If it fails, the delta might be the actual target content
		// This happens when GetDelta creates a blob for certain cases
		if len(baseContent) == 0 || diffs.Type() == plumbing.BlobObject {
			targetContent = deltaContent
		} else {
			return nil, err
		}
	}

	// Now use diff.Do to find differences
	changes := diff.Do(string(baseContent), string(targetContent))

	return changes, nil
}

// ApplyDiff applies diff changes to produce the final merged content
func (s *diffMergeService) ApplyDiff(baseContent string, changes []diffmatchpatch.Diff) ([]byte, error) {
	// Build result: start with base content bytes
	result := make([]byte, len(baseContent))
	copy(result, baseContent)

	// Track added content as bytes
	var additions [][]byte

	// Process the diff changes
	for _, change := range changes {
		switch change.Type {
		case diffmatchpatch.DiffInsert:
			// Collect additions as bytes
			additions = append(additions, []byte(change.Text))
		case diffmatchpatch.DiffDelete:
			// Skip deletions - we only want additions
			continue
		case diffmatchpatch.DiffEqual:
			// Skip unchanged parts
			continue
		}
	}

	// Append all additions to the base content
	if len(additions) > 0 {
		// Add newline if base doesn't end with one
		if len(result) > 0 && result[len(result)-1] != '\n' {
			result = append(result, '\n')
		}
		// Concatenate all additions using bytes.Join
		result = append(result, bytes.Join(additions, nil)...)
	}

	return bytes.Trim(result, "\x00"), nil
}

// PrettyPrint renders diff changes in a beautiful format using pterm (shows added lines only)
func (s *diffMergeService) PrettyPrint(diffs []diffmatchpatch.Diff) string {
	// Filter for added lines only
	var hasAdditions bool
	for _, diff := range diffs {
		if diff.Type == diffmatchpatch.DiffInsert {
			hasAdditions = true
			break
		}
	}

	if !hasAdditions {
		return pterm.Info.Sprint("No additions detected")
	}

	var builder strings.Builder
	lineNum := 1

	// Create styled renderers
	addStyle := pterm.NewStyle(pterm.FgGreen, pterm.BgDefault)
	lineNumStyle := pterm.NewStyle(pterm.FgCyan)

	// Header
	header := pterm.DefaultBox.WithTitle("Added Lines").Sprint("")
	builder.WriteString(header)
	builder.WriteString("\n\n")

	// Process only insertions
	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffInsert {
			continue
		}

		lines := strings.Split(diff.Text, "\n")

		for i, line := range lines {
			// Skip empty lines at the end of the diff text
			if i == len(lines)-1 && line == "" {
				continue
			}

			// Format line number
			lineNumStr := fmt.Sprintf("%4d │ ", lineNum)
			builder.WriteString(lineNumStyle.Sprint(lineNumStr))

			// Print the added line
			builder.WriteString(addStyle.Sprint("+ " + line))
			builder.WriteString("\n")
			lineNum++
		}
	}

	// Summary section
	var addCount int
	for _, diff := range diffs {
		if diff.Type == diffmatchpatch.DiffInsert {
			lineCount := strings.Count(diff.Text, "\n")
			if lineCount == 0 && diff.Text != "" {
				lineCount = 1
			}
			addCount += lineCount
		}
	}

	// Create summary
	builder.WriteString("\n")
	builder.WriteString(pterm.DefaultSection.Sprint("Summary"))
	summary := pterm.Success.Sprintf("Total lines added: %d", addCount)
	builder.WriteString(summary)

	return builder.String()
}
