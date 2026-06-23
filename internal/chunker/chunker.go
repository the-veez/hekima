// Package chunker splits East African documents into semantically
// meaningful chunks based on each document type's structural grammar.
//
// Generic chunkers ignore document structure. Hekima does not.
package chunker

import (
	"strings"

	"github.com/the-veez/hekima/internal/detector"
)

// Chunk is one semantically complete piece of a document.
// It carries not just the text but enough metadata for an AI retrieval
// system to understand where this chunk came from and what it means.
type Chunk struct {
	ID       int    `json:"id"`
	Text     string `json:"text"`
	Section  string `json:"section"`
	DocType  string `json:"doc_type"`
	Filename string `json:"filename"`
}

// minChunkLength is the minimum number of characters a chunk must have
// to be kept. Shorter chunks are usually noise — stray headings,
// page numbers, or whitespace artifacts.
const minChunkLength = 50

// Split is the main entry point. It routes a document to the correct
// splitting strategy based on its detected type.
func Split(doc detector.Document) []Chunk {
	switch doc.Type {
	case detector.TypeSACCOPolicy:
		return splitByHeadings(doc)
	case detector.TypeCBKCircular:
		return splitByNumberedSections(doc)
	case detector.TypeCourtJudgment:
		return splitByCourtMarkers(doc)
	default:
		return splitByParagraphs(doc)
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

	// Flush the final section
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
// Kenyan courts use consistent section headers across different courts.
func splitByCourtMarkers(doc detector.Document) []Chunk {
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

// splitByParagraphs is the fallback for unknown document types.
// Still better than fixed-size character splitting because it never
// cuts mid-sentence or mid-paragraph.
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
// Headings in East African policy documents tend to be short,
// mostly uppercase, and not ending with a period.
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
		if ch >= 'A' && ch <= 'Z' {
			upperCount++
			letterCount++
		} else if ch >= 'a' && ch <= 'z' {
			letterCount++
		}
	}

	if letterCount == 0 {
		return false
	}

	return float64(upperCount)/float64(letterCount) >= 0.6
}

// isNumberedSection returns true if a line starts with a section number
// like "1.", "2.", "1.1", "3.2.1" followed by content.
func isNumberedSection(line string) bool {
	if len(line) < 4 {
		return false
	}

	i := 0
	hasDigit := false
	for i < len(line) {
		ch := line[i]
		if ch >= '0' && ch <= '9' {
			hasDigit = true
			i++
		} else if ch == '.' && hasDigit {
			rest := strings.TrimSpace(line[i+1:])
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