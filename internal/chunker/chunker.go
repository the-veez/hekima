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
)

// ErrUnknownDocType is returned when the document type could not be
// identified. Callers must handle this explicitly — Hekima does not
// silently fall back to paragraph splitting.
var ErrUnknownDocType = errors.New("document type is unknown: detection failed or document type is unsupported")

const minChunkLength = 50

// partHeader matches Kenyan legislative Part headers in body text.
// Patterns handled:
//   - Part I – PRELIMINARY
//   - Part VIA – REGULATIONS OF FOREIGN EXCHANGE DEALINGS
//   - Part VIB – MORTGAGE FINANCING BUSINESS
//
// Roman numeral components: I, V, X, L, C, D, M (covers all Parts
// found in Kenyan Acts). Compound suffixes A, B, C, D handle sub-Parts
// introduced by amendment (Part VIA, VIB, VIC, VID in the CBK Act).
// The separator is an em dash (–) or hyphen (-) with surrounding spaces.
var partHeader = regexp.MustCompile(
	`(?i)^Part\s+[IVXLCDM]+(A|B|C|D)?\s+[–-]\s+\S`,
)

// sectionHeader matches top-level section numbers in Kenyan legislation.
// Patterns handled:
//   - "1. Short title"         — pure integer section
//   - "4A. Other objects"      — integer + uppercase letter(s) (amended section)
//   - "33U. Disclosure..."     — two-digit integer + uppercase letter
//   - "51B. Penalties..."      — two-digit integer + uppercase letter
//
// Explicitly excluded:
//   - "(1)", "(a)", "(i)"      — subsections/paragraphs, stay inside parent chunk
//   - "1.1 Requirement"        — decimal subsections (CBK circular style)
//
// The number must appear at the start of the trimmed line (after
// TrimSpace) with no leading spaces — this distinguishes body section
// headers from TOC entries which carry significant leading whitespace
// before pdftotext strips them via TrimSpace in the chunker loop.
var sectionHeader = regexp.MustCompile(
	`^(\d+[A-Z]*)\.\s+\S`,
)

// Split routes a document to the correct splitting strategy based on
// its detected type.
//
// Returns ErrUnknownDocType if the document type is unknown.
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
	case models.TypeLegislation:
		return splitLegislation(doc), nil
	case models.TypeUnknown:
		return nil, fmt.Errorf("%w: file=%q", ErrUnknownDocType, doc.Filename)
	default:
		return nil, fmt.Errorf("chunker: no splitting strategy registered for document type %q in file %q", doc.Type, doc.Filename)
	}
}

// splitLegislation chunks Kenyan Acts and Statutes by their structural
// grammar: Parts and Sections.
//
// Pre-processing: the Table of Contents is stripped before chunking.
// The TOC occupies the opening pages and contains the same Part and
// Section patterns as the body, but every entry carries dot-leaders
// ("......"). The body begins at the first Part header that has no
// dot-leaders — that line is the true structural start of the Act.
//
// Chunking strategy after TOC removal:
//   - Section headers create a new chunk boundary. This is the only
//     boundary that reliably produces a non-empty chunk, because every
//     section has body text under it before the next header.
//   - Part headers update currentPart (stored in every subsequent
//     chunk's Metadata["part"]) but do NOT reliably produce a
//     standalone chunk of their own. In real Acts, a Part header is
//     immediately followed by its first Section header with no body
//     text in between ("Part II – ESTABLISHMENT..." then directly
//     "3. Establishment of the Bank"), so accumulated text under the
//     Part header is empty and buildChunk discards it via
//     minChunkLength. This is intentional, not a bug: Part identity
//     is fully preserved via Metadata["part"] on every section beneath
//     it, and a RAG pipeline filtering or boosting by Part should read
//     that field rather than expect a dedicated Part-only chunk. The
//     rare exception is a Part with introductory prose before its
//     first numbered section (e.g. Part VIC in the CBK Act) — that
//     prose does form its own chunk, with Section equal to the Part
//     header text.
//   - Subsections (1), (2), (a), (i) accumulate inside their parent chunk.
func splitLegislation(doc models.Document) []models.Chunk {
	lines := strings.Split(doc.RawText, "\n")

	// Find the first body Part header — the line that matches a Part
	// header pattern AND contains no dot-leaders. Everything before this
	// line is preamble or TOC and is discarded.
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
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, currentPart, doc); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentPart = trimmed
			currentSection = trimmed
			currentLines = []string{}

		case isSectionHeader(trimmed) && !isTOCLine(trimmed):
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, currentPart, doc); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}

		default:
			currentLines = append(currentLines, line)
		}
	}

	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, currentPart, doc); ok {
		chunks = append(chunks, chunk)
	}

	return chunks
}

// isPartHeader returns true if the line is a legislative Part header.
// Examples: "Part I – PRELIMINARY", "Part VIA – REGULATIONS OF FOREIGN EXCHANGE DEALINGS"
func isPartHeader(line string) bool {
	return partHeader.MatchString(line)
}

// isSectionHeader returns true if the line begins with a legislative
// section number: integer, optionally followed by uppercase letters,
// then a dot and a space.
// Examples: "1. Short title", "4A. Other objects", "33U. Disclosure..."
//
// Note: repealed or spent sections (e.g. "33N. [Repealed by Act No. 9
// of 1996, s. 13.]", "33O. [Spent]") are correctly matched here but
// produce no output chunk — their accumulated body text is just the
// bracketed notice, which falls under minChunkLength in buildChunk and
// is discarded. There is no substantive legal content to retrieve
// from a repealed section, so this is the desired outcome.
func isSectionHeader(line string) bool {
	return sectionHeader.MatchString(line)
}

// isTOCLine returns true if the line is a Table of Contents entry
// rather than a body section header. TOC lines contain dot-leaders
// after the section title: "1. Short title .................. 1"
// We detect this by checking for a run of three or more consecutive
// dots anywhere in the line.
func isTOCLine(line string) bool {
	return strings.Contains(line, "...")
}

// buildChunk assembles a Chunk from accumulated lines.
// Returns false if the resulting text is too short to be useful
// (minChunkLength) — this is what causes most Part headers to be
// absorbed into metadata rather than becoming their own chunk; see
// splitLegislation's doc comment for why that's intentional.
// For legislation chunks, the Part name is stored in Metadata so
// retrieval systems can filter or boost by Part.
func buildChunk(id int, lines []string, section, part string, doc models.Document) (models.Chunk, bool) {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(text) < minChunkLength {
		return models.Chunk{}, false
	}

	metadata := map[string]string{
		"part": part,
	}

	return models.Chunk{
		ID:       id,
		Text:     text,
		Section:  section,
		DocType:  string(doc.Type),
		Filename: doc.Filename,
		Metadata: metadata,
	}, true
}

// splitByHeadings cuts at ALL-CAPS or Title Case headings.
// Used for SACCO policies where sections are labeled like:
// "ELIGIBILITY CRITERIA", "DEFAULT AND PENALTIES", etc.
func splitByHeadings(doc models.Document) []models.Chunk {
	var chunks []models.Chunk
	lines := strings.Split(doc.RawText, "\n")
	currentSection := "Introduction"
	var currentLines []string
	chunkID := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isHeading(trimmed) {
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// splitByNumberedSections cuts at top-level numbered section markers.
// Used for CBK circulars: "1. Purpose", "2. Scope", "3. Requirements".
// Subsections (3.1, 4.2) are intentionally excluded — they accumulate
// inside their parent section chunk.
func splitByNumberedSections(doc models.Document) []models.Chunk {
	var chunks []models.Chunk
	lines := strings.Split(doc.RawText, "\n")
	currentSection := "Preamble"
	var currentLines []string
	chunkID := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isNumberedSection(trimmed) {
			if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc); ok {
				chunks = append(chunks, chunk)
				chunkID++
			}
			currentSection = trimmed
			currentLines = []string{}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// splitByCourtMarkers cuts at recognized Kenyan court judgment markers.
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
				if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc); ok {
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
	if chunk, ok := buildChunk(chunkID, currentLines, currentSection, "", doc); ok {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// splitByParagraphs splits on blank lines. Used for land titles.
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

// isHeading returns true if a line looks like a SACCO policy heading.
// Uses unicode-aware character classification for non-ASCII text.
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

// isNumberedSection returns true if a line is a TOP-LEVEL CBK circular
// section: single integer followed by dot-space. Subsections (3.1, 4.2)
// are rejected — the digit after the dot excludes them.
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
