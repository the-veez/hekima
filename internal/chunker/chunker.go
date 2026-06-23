// Package chunker splits East African documents into semantically
// meaningful chunks based on each document type's structural grammar.
//
// The core insight: different document types have different structures.
// A SACCO policy is organized by headings. A CBK circular is organized
// by numbered sections. A court judgment is organized by legal markers
// like BACKGROUND, FINDINGS, and ORDER.
//
// Generic chunkers ignore this. Hekima does not.
package chunker

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/the-veez/hekima/internal/detector"
)

// ErrUnknownDocType is returned when the document type could not be
// identified by the detector. Callers must handle this explicitly —
// Hekima does not silently fall back to paragraph splitting, because
// silent fallback produces bad chunks with no observable signal of failure.
var ErrUnknownDocType = errors.New("document type is unknown: detection failed or document type is unsupported")

// Chunk is one semantically complete piece of a document.
// It carries not just the text but enough metadata for an AI retrieval
// system to understand where this chunk came from and what it means.
type Chunk struct {
	ID       int               `json:"id"`
	Text     string            `json:"text"`
	Section  string            `json:"section"`
	DocType  string            `json:"doc_type"`
	Filename string            `json:"filename"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// minChunkLength is the minimum number of characters a chunk must have
// to be kept. Shorter chunks are usually noise — stray headings,
// page numbers, or whitespace artifacts.
const minChunkLength = 50

// Split is the main entry point. It routes a document to the correct
// splitting strategy based on its detected type.
//
// Returns ErrUnknownDocType if the document type could not be identified.
// Callers must not treat an empty chunk slice as a success signal — always
// check the error first.
func Split(doc detector.Document) ([]Chunk, error) {
	switch doc.Type {
	case detector.TypeSACCOPolicy:
		return splitByHeadings(doc), nil
	case detector.TypeCBKCircular:
		return splitByNumberedSections(doc), nil
	case detector.TypeCourtJudgment:
		return splitByCourtMarkers(doc), nil
	case detector.TypeLandTitle:
		return splitByParagraphs(doc), nil
	case detector.TypeUnknown:
		return nil, fmt.Errorf("%w: file=%q", ErrUnknownDocType, doc.Filename)
	default:
		// This branch is reached only if a new DocType is added to the
		// detector without a corresponding case here. Fail loudly so the
		// gap is caught at development time, not in production.
		return nil, fmt.Errorf("chunker: no splitting strategy registered for document type %q in file %q", doc.Type, doc.Filename)
	}
}

// splitByHeadings cuts at ALL-CAPS or Title Case headings.
// Used for SACCO policies where sections are labeled like:
// "ELIGIBILITY CRITERIA", "DEFAULT AND PENALTIES", etc.
func splitByHeadings(doc detector.Document) []Chunk {
	var chunks []Chunk
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

	// Flush the final section.
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, doc); ok {
		chunks = append(chunks, chunk)
	}

	return chunks
}

// splitByNumberedSections cuts at numbered section markers like:
// "1. Purpose", "2. Scope", "3.1 Requirements"
// Used for CBK circulars which follow this structure consistently.
func splitByNumberedSections(doc detector.Document) []Chunk {
	var chunks []Chunk
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

// splitByCourtMarkers cuts at recognized Kenyan court judgment markers.
// Kenyan courts use consistent section headers like BACKGROUND, ISSUES,
// ANALYSIS, FINDINGS, and ORDER across different courts and judges.
func splitByCourtMarkers(doc detector.Document) []Chunk {
	// These are the structural markers found in Kenyan court judgments.
	// Ordered by typical appearance in a judgment.
	courtMarkers := []string{
		"BACKGROUND",
		"FACTS",
		"ISSUES FOR DETERMINATION",
		"ISSUES",
		"ANALYSIS",
		"FINDINGS",
		"DETERMINATION",
		"CONCLUSION",
		"ORDER",
		"COSTS",
	}

	var chunks []Chunk
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

// splitByParagraphs splits on blank lines, respecting paragraph boundaries.
// Used for land titles. This is still meaningfully better than fixed-size
// character splitting because it never cuts mid-sentence or mid-paragraph.
func splitByParagraphs(doc detector.Document) []Chunk {
	var chunks []Chunk
	paragraphs := strings.Split(doc.RawText, "\n\n")

	for i, para := range paragraphs {
		trimmed := strings.TrimSpace(para)
		if len(trimmed) >= minChunkLength {
			chunks = append(chunks, Chunk{
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

// buildChunk assembles a Chunk from accumulated lines.
// Returns false if the resulting text is too short to be useful.
func buildChunk(id int, lines []string, section string, doc detector.Document) (Chunk, bool) {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(text) < minChunkLength {
		return Chunk{}, false
	}
	return Chunk{
		ID:       id,
		Text:     text,
		Section:  section,
		DocType:  string(doc.Type),
		Filename: doc.Filename,
	}, true
}

// isHeading returns true if a line looks like a document section heading.
// Headings in East African policy documents tend to be:
//   - Short (between 3 and 80 characters)
//   - Mostly or fully uppercase letters (≥60% of letter characters)
//   - Not ending with a period or comma (that would make it a sentence)
//   - Not consisting entirely of digits or punctuation
//
// Uses unicode-aware character classification to correctly handle
// Swahili and other non-ASCII text found in East African documents.
func isHeading(line string) bool {
	if len(line) < 3 || len(line) > 80 {
		return false
	}
	if strings.HasSuffix(line, ".") || strings.HasSuffix(line, ",") {
		return false
	}

	upperCount := 0
	letterCount := 0
	for _, ch := range line { // range over string yields runes, not bytes
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

// isNumberedSection returns true if a line starts with a section number
// like "1.", "2.", "1.1", "3.2.1" followed by more content.
//
// Uses rune-based iteration to correctly handle non-ASCII characters
// that may appear in East African document text.
func isNumberedSection(line string) bool {
	if len(line) < 4 {
		return false
	}

	runes := []rune(line)
	i := 0
	hasDigit := false

	for i < len(runes) {
		ch := runes[i]
		if unicode.IsDigit(ch) {
			hasDigit = true
			i++
		} else if ch == '.' && hasDigit {
			rest := strings.TrimSpace(string(runes[i+1:]))
			if len(rest) > 1 {
				return true
			}
			i++
		} else {
			break
		}
	}

	return false
}
