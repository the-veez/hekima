// Package chunker splits East African documents into semantically
// meaningful chunks based on each document type's structural grammar.
//
// The core insight: different document types have different structures.
// A SACCO policy is organized by headings. A CBK circular is organized
// by numbered sections. A court judgment is organized by legal markers.
// Kenyan legislation is organized by Parts and Sections with alphanumeric
// numbering (33A, 46A, 51B) common in amended Acts.
//
// Generic chunkers ignore this. Hekima does not.
package chunker

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/the-veez/hekima/internal/models"
	"github.com/the-veez/hekima/internal/tokenizer"
)

// ErrUnknownDocType is returned when the document type could not be
// identified. Callers must handle this explicitly — Hekima does not
// silently fall back to paragraph splitting.
var ErrUnknownDocType = errors.New("document type is unknown: detection failed or document type is unsupported")

const minChunkLength = 50

// Options controls optional behaviours of SplitWithOptions.
// The zero value is safe: no overlap, whitespace-based token counting.
type Options struct {
	// OverlapWords is the number of words to repeat at the start of each
	// chunk from the end of the previous chunk. Overlap preserves sentence
	// context that would otherwise be severed at a chunk boundary — a
	// clause that begins in chunk N and concludes in chunk N+1 remains
	// readable in both.
	//
	// Word boundaries are used (not BPE tokens) because the overlap
	// extraction iterates the previous chunk's text directly. For typical
	// East African regulatory prose, the difference from exact BPE-token
	// overlap is negligible.
	//
	// 0 disables overlap. Negative values are treated as 0.
	OverlapWords int

	// Tokenizer is used to populate TokenCount on every emitted chunk.
	// If nil, WhitespaceTokenizer is used (word count × 1.3, rounded up).
	// Swap in a BPETokenizer for exact counts when poppler-utils and
	// network access are available.
	Tokenizer tokenizer.Tokenizer
}

// partHeader matches Kenyan legislative Part headers in body text.
// Patterns handled:
//   - Part I – PRELIMINARY
//   - Part VIA – REGULATIONS OF FOREIGN EXCHANGE DEALINGS
//   - Part VIB – MORTGAGE FINANCING BUSINESS
var partHeader = regexp.MustCompile(
	`(?i)^Part\s+[IVXLCDM]+(A|B|C|D)?\s+[–-]\s+\S`,
)

// sectionHeader matches top-level section numbers in Kenyan legislation.
// Patterns handled: "1. Short title", "4A. Other objects", "33U. Disclosure..."
// Explicitly excluded: "(1)", "(a)", "(i)" subsections and "1.1" decimal subsections.
var sectionHeader = regexp.MustCompile(
	`^(\d+[A-Z]*)\.\s+\S`,
)

// Split routes a document to the correct splitting strategy using default
// options (no overlap, whitespace tokenizer). It exists so existing call
// sites need no changes.
func Split(doc models.Document) ([]models.Chunk, error) {
	return SplitWithOptions(doc, Options{})
}

// SplitWithOptions routes a document to the correct splitting strategy
// and applies the provided options (overlap, tokenizer).
//
// Returns ErrUnknownDocType if the document type is unknown.
func SplitWithOptions(doc models.Document, opts Options) ([]models.Chunk, error) {
	tok := resolveTokenizer(opts.Tokenizer)

	var chunks []models.Chunk
	switch doc.Type {
	case models.TypeSACCOPolicy:
		chunks = splitByHeadings(doc, tok)
	case models.TypeCBKCircular:
		chunks = splitByNumberedSections(doc, tok)
	case models.TypeCourtJudgment:
		chunks = splitByCourtMarkers(doc, tok)
	case models.TypeLandTitle:
		chunks = splitByParagraphs(doc, tok)
	case models.TypeLegislation:
		chunks = splitLegislation(doc, tok)
	case models.TypeUnknown:
		return nil, fmt.Errorf("%w: file=%q", ErrUnknownDocType, doc.Filename)
	default:
		return nil, fmt.Errorf("chunker: no splitting strategy registered for document type %q in file %q", doc.Type, doc.Filename)
	}

	if opts.OverlapWords > 0 && len(chunks) > 1 {
		chunks = applyOverlap(chunks, opts.OverlapWords)
	}
	return chunks, nil
}

// resolveTokenizer returns opts.Tokenizer if set, otherwise the
// zero-dependency WhitespaceTokenizer.
func resolveTokenizer(tok tokenizer.Tokenizer) tokenizer.Tokenizer {
	if tok != nil {
		return tok
	}
	return tokenizer.WhitespaceTokenizer{}
}

// applyOverlap prepends the last n words of chunk[i-1] to chunk[i].
// It is applied as a single post-processing pass after splitting so
// the logic is written once and shared across all document types.
//
// IDs are reassigned after overlap so they remain sequential.
// TokenCount is recalculated for each chunk after its text grows.
func applyOverlap(chunks []models.Chunk, overlapWords int) []models.Chunk {
	tok := tokenizer.WhitespaceTokenizer{}
	result := make([]models.Chunk, len(chunks))
	result[0] = chunks[0]

	for i := 1; i < len(chunks); i++ {
		tail := lastNWords(chunks[i-1].Text, overlapWords)
		if tail != "" {
			chunks[i].Text = tail + "\n" + chunks[i].Text
			chunks[i].TokenCount = tok.Count(chunks[i].Text)
		}
		result[i] = chunks[i]
	}
	return result
}

// lastNWords returns the last n whitespace-separated words of text,
// joined by single spaces. Returns empty string if text is empty or n <= 0.
func lastNWords(text string, n int) string {
	if n <= 0 || text == "" {
		return ""
	}
	words := strings.Fields(text)
	if len(words) <= n {
		return text
	}
	return strings.Join(words[len(words)-n:], " ")
}

// splitLegislation chunks Kenyan Acts and Statutes by their structural
// grammar: Parts and Sections.
//
// TOC stripping: the Table of Contents is discarded before chunking.
// The body begins at the first Part header with no dot-leaders.
//
// Part headers update currentPart but rarely produce a standalone chunk:
// in real Acts a Part header is immediately followed by its first Section
// header with no body text between them, so accumulated text is empty and
// buildChunk discards it via minChunkLength. Part identity is preserved
// via Metadata["part"] on every section beneath it.
func splitLegislation(doc models.Document, tok tokenizer.Tokenizer) []models.Chunk {
	lines := strings.Split(doc.RawText, "\n")

	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isPartHeader(trimmed) && !isTOCLine(trimmed) {
			bodyStart = i
			break
		}
	}

	var chunks []models.Chunk
	currentPart := "Preamble"
	currentSection := "Preamble"
	var currentLines []string
	chunkID := 0

	for _, line := range lines[bodyStart:] {
		trimmed := strings.TrimSpace(line)
		switch {
		case isPartHeader(trimmed) && !isTOCLine(trimmed):
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, currentPart, doc, tok); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentPart = trimmed
			currentSection = trimmed
			currentLines = []string{}
		case isSectionHeader(trimmed) && !isTOCLine(trimmed):
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, currentPart, doc, tok); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}
		default:
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, currentPart, doc, tok); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func isPartHeader(line string) bool {
	return partHeader.MatchString(line)
}

func isSectionHeader(line string) bool {
	return sectionHeader.MatchString(line)
}

// isTOCLine returns true if the line is a Table of Contents entry.
// TOC lines contain dot-leaders: "1. Short title .................. 1"
func isTOCLine(line string) bool {
	return strings.Contains(line, "...")
}

// buildChunk assembles a Chunk from accumulated lines.
// Returns false if the resulting text is too short to be useful.
// For legislation chunks, the Part name is stored in Metadata.
func buildChunk(id int, lines []string, section, part string, doc models.Document, tok tokenizer.Tokenizer) (models.Chunk, bool) {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(text) < minChunkLength {
		return models.Chunk{}, false
	}
	metadata := map[string]string{
		"part": part,
	}
	return models.Chunk{
		ID:         id,
		Text:       text,
		Section:    section,
		DocType:    string(doc.Type),
		Filename:   doc.Filename,
		TokenCount: tok.Count(text),
		Metadata:   metadata,
	}, true
}

// splitByHeadings cuts at ALL-CAPS or Title Case headings.
// Used for SACCO policies.
func splitByHeadings(doc models.Document, tok tokenizer.Tokenizer) []models.Chunk {
	var chunks []models.Chunk
	lines := strings.Split(doc.RawText, "\n")
	currentSection := "Introduction"
	var currentLines []string
	chunkID := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isHeading(trimmed) {
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc, tok); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc, tok); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// splitByNumberedSections cuts at top-level numbered section markers.
// Used for CBK circulars. Subsections (3.1, 4.2) stay inside parent chunks.
func splitByNumberedSections(doc models.Document, tok tokenizer.Tokenizer) []models.Chunk {
	var chunks []models.Chunk
	lines := strings.Split(doc.RawText, "\n")
	currentSection := "Preamble"
	var currentLines []string
	chunkID := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isNumberedSection(trimmed) {
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc, tok); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc, tok); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// splitByCourtMarkers cuts at recognized Kenyan court judgment markers.
func splitByCourtMarkers(doc models.Document, tok tokenizer.Tokenizer) []models.Chunk {
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
				if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc, tok); ok {
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
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc, tok); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// splitByParagraphs splits on blank lines. Used for land titles.
// chunkID is a dedicated counter (not the paragraph slice index) so
// IDs are always sequential even when short paragraphs are skipped.
func splitByParagraphs(doc models.Document, tok tokenizer.Tokenizer) []models.Chunk {
	var chunks []models.Chunk
	paragraphs := strings.Split(doc.RawText, "\n\n")
	chunkID := 0
	for _, para := range paragraphs {
		trimmed := strings.TrimSpace(para)
		if len(trimmed) >= minChunkLength {
			chunks = append(chunks, models.Chunk{
				ID:         chunkID,
				Text:       trimmed,
				Section:    "Unknown",
				DocType:    string(doc.Type),
				Filename:   doc.Filename,
				TokenCount: tok.Count(trimmed),
				Metadata:   map[string]string{},
			})
			chunkID++
		}
	}
	return chunks
}

// isHeading returns true if a line looks like a SACCO policy heading.
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

// isNumberedSection returns true if a line is a top-level CBK circular
// section. Subsections (3.1, 4.2) are rejected.
func isNumberedSection(line string) bool {
	if len(line) < 4 {
		return false
	}
	runes := []rune(line)
	i := 0
	if !unicode.IsDigit(runes[i]) {
		return false
	}
	for i < len(runes) && unicode.IsDigit(runes[i]) {
		i++
	}
	if i >= len(runes) || runes[i] != '.' {
		return false
	}
	i++
	if i >= len(runes) || unicode.IsDigit(runes[i]) {
		return false
	}
	rest := strings.TrimSpace(string(runes[i:]))
	return len(rest) > 1
}
