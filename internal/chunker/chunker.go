// Package chunker splits East African documents into semantically
// meaningful chunks based on each document type's structural grammar.
package chunker

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/the-veez/hekima/internal/models"
)

// ErrUnknownDocType is returned when the document type could not be
// identified. Callers must handle this explicitly — Hekima does not
// silently fall back to paragraph splitting.
var ErrUnknownDocType = errors.New("document type is unknown: detection failed or document type is unsupported")

const minChunkLength = 50

// Split routes a document to the correct splitting strategy based on
// its detected type.
//
// Returns ErrUnknownDocType if the document type is unknown.
// An empty chunk slice with a nil error indicates a valid but very
// short document — callers should handle this case.
func Split(doc models.Document) ([]models.Chunk, error) {
	switch doc.Type {
	case models.TypeSACCOPolicy:
		return splitByHeadings(doc), nil
	case models.TypeCBKCircular:
		return splitByNumberedSections(doc), nil
	case models.TypeCourtJudgment:
		return splitByCourtMarkers(doc), nil
	case models.TypeLandTitle:
		return splitByParagraphs(doc), nil
	case models.TypeUnknown:
		return nil, fmt.Errorf("%w: file=%q", ErrUnknownDocType, doc.Filename)
	default:
		// A new DocType was added to models without a corresponding case here.
		return nil, fmt.Errorf("chunker: no splitting strategy registered for document type %q in file %q", doc.Type, doc.Filename)
	}
}

func splitByHeadings(doc models.Document) []models.Chunk {
	var chunks []models.Chunk
	lines := strings.Split(doc.RawText, "\n")
	currentSection := "Introduction"
	var currentLines []string
	chunkID := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isHeading(trimmed) {
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, doc); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, doc); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func splitByNumberedSections(doc models.Document) []models.Chunk {
	var chunks []models.Chunk
	lines := strings.Split(doc.RawText, "\n")
	currentSection := "Preamble"
	var currentLines []string
	chunkID := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isNumberedSection(trimmed) {
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, doc); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, doc); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func splitByCourtMarkers(doc models.Document) []models.Chunk {
	courtMarkers := []string{
		"BACKGROUND", "FACTS", "ISSUES FOR DETERMINATION", "ISSUES",
		"ANALYSIS", "FINDINGS", "DETERMINATION", "CONCLUSION", "ORDER", "COSTS",
	}

	var chunks []models.Chunk
	lines := strings.Split(doc.RawText, "\n")
	currentSection := "HEADER"
	var currentLines []string
	chunkID := 0

	for _, line := range lines {
		upperLine := strings.ToUpper(strings.TrimSpace(line))
		matched := false
		for _, marker := range courtMarkers {
			if upperLine == marker || strings.HasPrefix(upperLine, marker) {
				if chunk, ok := buildChunk(chunkID, currentLines, currentSection, doc); ok {
					chunks = append(chunks, chunk)
					chunkID++
				}
				currentSection = strings.TrimSpace(line)
				currentLines = []string{}
				matched = true
				break
			}
		}
		if !matched {
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, doc); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func splitByParagraphs(doc models.Document) []models.Chunk {
	var chunks []models.Chunk
	paragraphs := strings.Split(doc.RawText, "\n\n")
	for i, para := range paragraphs {
		trimmed := strings.TrimSpace(para)
		if len(trimmed) >= minChunkLength {
			chunks = append(chunks, models.Chunk{
				ID:       i,
				Text:     trimmed,
				Section:  "Unknown",
				DocType:  string(doc.Type),
				Filename: doc.Filename,
			})
		}
	}
	return chunks
}

func buildChunk(id int, lines []string, section string, doc models.Document) (models.Chunk, bool) {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(text) < minChunkLength {
		return models.Chunk{}, false
	}
	return models.Chunk{
		ID:       id,
		Text:     text,
		Section:  section,
		DocType:  string(doc.Type),
		Filename: doc.Filename,
	}, true
}

func isHeading(line string) bool {
	if len(line) < 3 || len(line) > 80 {
		return false
	}
	if strings.HasSuffix(line, ".") || strings.HasSuffix(line, ",") {
		return false
	}
	upperCount := 0
	letterCount := 0
	for _, ch := range line {
		if unicode.IsUpper(ch) {
			upperCount++
			letterCount++
		} else if unicode.IsLower(ch) {
			letterCount++
		}
	}
	if letterCount == 0 {
		return false
	}
	return float64(upperCount)/float64(letterCount) >= 0.6
}

// isNumberedSection returns true if a line is a TOP-LEVEL numbered section
// header: "1. Title", "2. Title", etc.
//
// Subsection headers like "3.1 Requirement" and "4.2.1 Detail" are
// intentionally excluded — they belong inside their parent section's chunk,
// not as independent chunk boundaries. Splitting at subsections produces
// chunks that are too small and lose the parent section context.
//
// Uses rune-based iteration to correctly handle non-ASCII characters.
func isNumberedSection(line string) bool {
	if len(line) < 4 {
		return false
	}

	runes := []rune(line)
	i := 0

	// Consume the leading integer (one or more digits).
	if !unicode.IsDigit(runes[i]) {
		return false
	}
	for i < len(runes) && unicode.IsDigit(runes[i]) {
		i++
	}

	// Must be followed immediately by a dot.
	if i >= len(runes) || runes[i] != '.' {
		return false
	}
	i++ // consume the dot

	// The character after the dot must be a space (top-level: "1. Title").
	// If it is another digit, this is a subsection ("1.1 Detail") — reject it.
	if i >= len(runes) || unicode.IsDigit(runes[i]) {
		return false
	}

	// There must be meaningful content after the dot and space.
	rest := strings.TrimSpace(string(runes[i:]))
	return len(rest) > 1
}
