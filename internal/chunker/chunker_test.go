package chunker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/the-veez/hekima/internal/chunker"
	"github.com/the-veez/hekima/internal/models"
)

func loadTestData(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadTestData: cannot read %s: %v", path, err)
	}
	return string(data)
}

func makeDoc(filename string, docType models.DocumentType, text string) models.Document {
	return models.Document{
		Filename: filename,
		Type:     docType,
		RawText:  text,
	}
}

// TestSplit_UnknownTypeReturnsError verifies that TypeUnknown is never
// silently swallowed — callers must receive an explicit error.
func TestSplit_UnknownTypeReturnsError(t *testing.T) {
	doc := makeDoc("mystery.txt", models.TypeUnknown, "some content here that is long enough")
	_, err := chunker.Split(doc)
	if err == nil {
		t.Fatal("Split with TypeUnknown: expected error, got nil")
	}
}

func TestSplit_CBKCircular(t *testing.T) {
	text := loadTestData(t, "cbk_circular.txt")
	doc := makeDoc("cbk_circular.txt", models.TypeCBKCircular, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("Split(cbk_circular): unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("Split(cbk_circular): got 0 chunks, want > 0")
	}

	// The CBK circular has 7 numbered sections (1–7). Every top-level
	// section must appear as a chunk boundary. This test will currently
	// FAIL because sections 3, 4, and 5 are not being detected —
	// that failure is the signal to fix isNumberedSection().
	expectedSections := []string{
		"1. Purpose",
		"2. Scope",
		"3. Customer Data Protection Requirements",
		"4. Lending Conduct Requirements",
		"5. Reporting Requirements",
		"6. Compliance Timeline",
		"7. Penalties for Non-Compliance",
	}

	sectionFound := make(map[string]bool)
	for _, chunk := range chunks {
		for _, expected := range expectedSections {
			if strings.HasPrefix(chunk.Section, expected) {
				sectionFound[expected] = true
			}
		}
	}

	for _, section := range expectedSections {
		if !sectionFound[section] {
			t.Errorf("Split(cbk_circular): section %q was not found as a chunk boundary", section)
		}
	}
}

func TestSplit_SACCOPolicy(t *testing.T) {
	text := loadTestData(t, "sacco_policy.txt")
	doc := makeDoc("sacco_policy.txt", models.TypeSACCOPolicy, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("Split(sacco_policy): unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("Split(sacco_policy): got 0 chunks, want > 0")
	}

	// DEFAULT AND PENALTIES must be a separate chunk from LOAN DISBURSEMENT.
	// If they are merged, the penalty clauses lose their section context —
	// exactly the failure mode Hekima exists to prevent.
	penaltyChunk := -1
	disbursementChunk := -1
	for i, chunk := range chunks {
		if chunk.Section == "DEFAULT AND PENALTIES" {
			penaltyChunk = i
		}
		if chunk.Section == "LOAN DISBURSEMENT" {
			disbursementChunk = i
		}
	}

	if penaltyChunk == -1 {
		t.Error("Split(sacco_policy): 'DEFAULT AND PENALTIES' section not found as a chunk boundary")
	}
	if disbursementChunk == -1 {
		t.Error("Split(sacco_policy): 'LOAN DISBURSEMENT' section not found as a chunk boundary")
	}
	if penaltyChunk != -1 && disbursementChunk != -1 && penaltyChunk == disbursementChunk {
		t.Error("Split(sacco_policy): DEFAULT AND PENALTIES and LOAN DISBURSEMENT landed in the same chunk")
	}
}

func TestSplit_CourtJudgment(t *testing.T) {
	text := loadTestData(t, "court_judgment.txt")
	doc := makeDoc("court_judgment.txt", models.TypeCourtJudgment, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("Split(court_judgment): unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("Split(court_judgment): got 0 chunks, want > 0")
	}

	// The ratio decidendi (ANALYSIS/FINDINGS) must be separate from the
	// ORDER — merging them makes retrieval of the legal reasoning ambiguous.
	requiredSections := []string{"BACKGROUND", "FACTS", "ANALYSIS", "FINDINGS", "ORDER"}
	sectionFound := make(map[string]bool)
	for _, chunk := range chunks {
		upper := strings.ToUpper(chunk.Section)
		for _, required := range requiredSections {
			if strings.HasPrefix(upper, required) {
				sectionFound[required] = true
			}
		}
	}
	for _, section := range requiredSections {
		if !sectionFound[section] {
			t.Errorf("Split(court_judgment): required section %q not found as a chunk boundary", section)
		}
	}
}

func TestSplit_ChunkMetadataIsComplete(t *testing.T) {
	text := loadTestData(t, "sacco_policy.txt")
	doc := makeDoc("sacco_policy.txt", models.TypeSACCOPolicy, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, chunk := range chunks {
		if chunk.Text == "" {
			t.Errorf("chunk %d: Text is empty", chunk.ID)
		}
		if chunk.Section == "" {
			t.Errorf("chunk %d: Section is empty", chunk.ID)
		}
		if chunk.DocType == "" {
			t.Errorf("chunk %d: DocType is empty", chunk.ID)
		}
		if chunk.Filename == "" {
			t.Errorf("chunk %d: Filename is empty", chunk.ID)
		}
	}
}

func TestSplit_ChunkIDsAreSequential(t *testing.T) {
	text := loadTestData(t, "sacco_policy.txt")
	doc := makeDoc("sacco_policy.txt", models.TypeSACCOPolicy, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, chunk := range chunks {
		if chunk.ID != i {
			t.Errorf("chunk at index %d has ID %d: IDs must be sequential from 0", i, chunk.ID)
		}
	}
}

// TestSplit_Legislation_PartAndSectionBoundaries verifies the core
// legislative chunking grammar against a small synthetic Act covering
// every structural case the real CBK Act exhibits: multiple Parts,
// alphanumeric sections (3A, 4A, 4B), a repealed section with no
// retrievable content, and a Part with introductory prose before its
// first numbered section.
func TestSplit_Legislation_PartAndSectionBoundaries(t *testing.T) {
	text := loadTestData(t, "sample_act.txt")
	doc := makeDoc("sample_act.txt", models.TypeLegislation, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("Split(sample_act): unexpected error: %v", err)
	}

	const wantChunks = 6
	if len(chunks) != wantChunks {
		t.Fatalf("Split(sample_act): got %d chunks, want %d", len(chunks), wantChunks)
	}

	// "1. Short title" must NOT appear as its own chunk: its body text
	// ("This Act may be cited as the Sample Act.") is shorter than
	// minChunkLength. This is the same rule that drops repealed
	// sections — too short to carry retrievable content.
	for _, c := range chunks {
		if c.Section == "1. Short title" {
			t.Error(`Split(sample_act): "1. Short title" produced a chunk, but its body text is below minChunkLength and should have been dropped`)
		}
	}

	// "4A. [Repealed...]" must never appear as a chunk boundary's
	// content survives — a repealed section has no substantive legal
	// content to retrieve.
	for _, c := range chunks {
		if strings.Contains(c.Text, "Repealed by Act No. 5 of 2010") {
			t.Error("Split(sample_act): repealed section 4A produced a retrievable chunk, but it should have been dropped (no substantive content below minChunkLength)")
		}
	}

	// Part I has no introductory prose before "1. Short title" — it
	// must NOT produce its own chunk. Its identity must still be
	// recoverable via Metadata["part"] on every section beneath it.
	for _, c := range chunks {
		if c.Section == "Part I – PRELIMINARY" {
			t.Error(`Split(sample_act): "Part I – PRELIMINARY" produced its own chunk, but it has no intro prose and should have been absorbed into the following section's metadata`)
		}
	}

	// Part II DOES have introductory prose before "3. Establishment of
	// the Authority" — it is the documented rare exception and MUST
	// produce its own chunk, with Section equal to the Part header.
	foundPartIIChunk := false
	for _, c := range chunks {
		if c.Section == "Part II – ESTABLISHMENT" {
			foundPartIIChunk = true
			if !strings.Contains(c.Text, "body corporate") {
				t.Errorf(`Split(sample_act): "Part II" chunk has wrong text: %q`, c.Text)
			}
		}
	}
	if !foundPartIIChunk {
		t.Error(`Split(sample_act): expected "Part II – ESTABLISHMENT" to produce its own chunk (it has intro prose before its first section), but no such chunk was found`)
	}

	// Every chunk must carry the correct Part in Metadata, regardless
	// of whether that Part produced its own chunk. This is the
	// mechanism a RAG pipeline relies on to filter or boost by Part.
	wantPartForSection := map[string]string{
		"2. Interpretation":                 "Part I – PRELIMINARY",
		"3. Establishment of the Authority": "Part II – ESTABLISHMENT",
		"3A. Powers of the Authority":       "Part II – ESTABLISHMENT",
		"4. Funds of the Authority":         "Part III – FINANCE",
		"4B. Annual estimates":              "Part III – FINANCE",
	}
	for _, c := range chunks {
		wantPart, ok := wantPartForSection[c.Section]
		if !ok {
			continue
		}
		if c.Metadata == nil {
			t.Errorf("chunk %q: Metadata is nil, want part %q", c.Section, wantPart)
			continue
		}
		if got := c.Metadata["part"]; got != wantPart {
			t.Errorf("chunk %q: Metadata[\"part\"] = %q, want %q", c.Section, got, wantPart)
		}
	}
}

// TestSplit_Legislation_AlphanumericSections verifies that section
// numbers with letter suffixes (3A, 4A, 4B) — the numbering scheme
// used throughout amended Kenyan Acts — are recognised as top-level
// section boundaries, not folded into the preceding section.
func TestSplit_Legislation_AlphanumericSections(t *testing.T) {
	text := loadTestData(t, "sample_act.txt")
	doc := makeDoc("sample_act.txt", models.TypeLegislation, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("Split(sample_act): unexpected error: %v", err)
	}

	wantSections := []string{
		"3A. Powers of the Authority",
		"4B. Annual estimates",
	}
	found := make(map[string]bool)
	for _, c := range chunks {
		for _, want := range wantSections {
			if c.Section == want {
				found[want] = true
			}
		}
	}
	for _, want := range wantSections {
		if !found[want] {
			t.Errorf("Split(sample_act): alphanumeric section %q was not found as its own chunk boundary", want)
		}
	}

	// "3. Establishment of the Authority" and "3A. Powers of the
	// Authority" must land in DIFFERENT chunks — if 3A is folded into
	// section 3's chunk, the powers clause loses its own section
	// identity, exactly the failure mode Hekima exists to prevent.
	var idx3, idx3A = -1, -1
	for i, c := range chunks {
		if c.Section == "3. Establishment of the Authority" {
			idx3 = i
		}
		if c.Section == "3A. Powers of the Authority" {
			idx3A = i
		}
	}
	if idx3 == -1 || idx3A == -1 {
		t.Fatal("Split(sample_act): could not locate both section 3 and 3A to compare")
	}
	if idx3 == idx3A {
		t.Error("Split(sample_act): section 3 and 3A landed in the same chunk")
	}
}

// TestSplit_Legislation_TOCIsStripped verifies that no chunk's text or
// section label originates from the Table of Contents. TOC entries
// share the same Part/Section patterns as the body but are marked by
// dot-leaders, and must never leak into chunk output.
func TestSplit_Legislation_TOCIsStripped(t *testing.T) {
	text := loadTestData(t, "sample_act.txt")
	doc := makeDoc("sample_act.txt", models.TypeLegislation, text)

	chunks, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("Split(sample_act): unexpected error: %v", err)
	}

	for _, c := range chunks {
		if strings.Contains(c.Text, "...") {
			t.Errorf("chunk %d (%q): contains dot-leaders, suggesting TOC content leaked into the body: %q", c.ID, c.Section, c.Text)
		}
		if strings.Contains(c.Section, "...") {
			t.Errorf("chunk %d: Section %q contains dot-leaders, suggesting a TOC line was treated as a body header", c.ID, c.Section)
		}
	}
}

// --- SplitWithOptions tests ---

// TestSplitWithOptions_TokenCountIsAlwaysPopulated verifies that every
// chunk carries a non-zero TokenCount when the document has real content.
// A zero count for a non-empty chunk would silently break any downstream
// pipeline that uses TokenCount to enforce context window limits.
func TestSplitWithOptions_TokenCountIsAlwaysPopulated(t *testing.T) {
	text := loadTestData(t, "cbk_circular.txt")
	doc := makeDoc("cbk_circular.txt", models.TypeCBKCircular, text)
	chunks, err := chunker.SplitWithOptions(doc, chunker.Options{})
	if err != nil {
		t.Fatalf("SplitWithOptions: unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("SplitWithOptions: got 0 chunks")
	}
	for _, c := range chunks {
		if c.TokenCount == 0 {
			t.Errorf("chunk %d (%q): TokenCount is 0 for non-empty chunk (text length %d)", c.ID, c.Section, len(c.Text))
		}
	}
}

// TestSplitWithOptions_ZeroOverlapMatchesSplit verifies that
// SplitWithOptions with a zero-value Options{} produces identical
// output to Split — the thin wrapper contract must hold exactly.
func TestSplitWithOptions_ZeroOverlapMatchesSplit(t *testing.T) {
	text := loadTestData(t, "sacco_policy.txt")
	doc := makeDoc("sacco_policy.txt", models.TypeSACCOPolicy, text)

	baseline, err := chunker.Split(doc)
	if err != nil {
		t.Fatalf("Split: unexpected error: %v", err)
	}
	withOpts, err := chunker.SplitWithOptions(doc, chunker.Options{})
	if err != nil {
		t.Fatalf("SplitWithOptions: unexpected error: %v", err)
	}
	if len(baseline) != len(withOpts) {
		t.Fatalf("chunk count mismatch: Split=%d SplitWithOptions=%d", len(baseline), len(withOpts))
	}
	for i := range baseline {
		if baseline[i].Text != withOpts[i].Text {
			t.Errorf("chunk %d text differs between Split and SplitWithOptions", i)
		}
		if baseline[i].Section != withOpts[i].Section {
			t.Errorf("chunk %d section differs: Split=%q SplitWithOptions=%q", i, baseline[i].Section, withOpts[i].Section)
		}
	}
}

// TestSplitWithOptions_OverlapWordsPrependsContext verifies the core
// overlap contract: the first N words of chunk[i].Text (after overlap)
// must equal the last N words of chunk[i-1].Text (before overlap).
//
// This is tested against a CBK circular because its numbered sections
// produce clean, predictable chunk boundaries.
func TestSplitWithOptions_OverlapWordsPrependsContext(t *testing.T) {
	text := loadTestData(t, "cbk_circular.txt")
	doc := makeDoc("cbk_circular.txt", models.TypeCBKCircular, text)

	const overlapWords = 10
	chunks, err := chunker.SplitWithOptions(doc, chunker.Options{OverlapWords: overlapWords})
	if err != nil {
		t.Fatalf("SplitWithOptions: unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatal("need at least 2 chunks to test overlap")
	}

	// Get the baseline (no overlap) to extract the "previous chunk" tail.
	baseline, _ := chunker.Split(doc)

	for i := 1; i < len(chunks); i++ {
		prevWords := strings.Fields(baseline[i-1].Text)
		var wantTail []string
		if len(prevWords) <= overlapWords {
			wantTail = prevWords
		} else {
			wantTail = prevWords[len(prevWords)-overlapWords:]
		}
		wantPrefix := strings.Join(wantTail, " ")

		got := strings.Fields(chunks[i].Text)
		if len(got) < len(wantTail) {
			t.Errorf("chunk %d: text too short to contain overlap prefix", i)
			continue
		}
		gotPrefix := strings.Join(got[:len(wantTail)], " ")
		if gotPrefix != wantPrefix {
			t.Errorf("chunk %d: overlap prefix = %q, want %q", i, gotPrefix, wantPrefix)
		}
	}
}

// TestSplitWithOptions_OverlapDoesNotAffectFirstChunk verifies that
// chunk 0 is never modified by the overlap pass — there is no previous
// chunk to draw context from.
func TestSplitWithOptions_OverlapDoesNotAffectFirstChunk(t *testing.T) {
	text := loadTestData(t, "cbk_circular.txt")
	doc := makeDoc("cbk_circular.txt", models.TypeCBKCircular, text)

	baseline, _ := chunker.Split(doc)
	withOverlap, err := chunker.SplitWithOptions(doc, chunker.Options{OverlapWords: 20})
	if err != nil {
		t.Fatalf("SplitWithOptions: unexpected error: %v", err)
	}
	if baseline[0].Text != withOverlap[0].Text {
		t.Errorf("chunk 0 was modified by overlap pass: got %q, want %q", withOverlap[0].Text, baseline[0].Text)
	}
}

// TestSplitWithOptions_NegativeOverlapTreatedAsZero verifies that a
// negative OverlapWords value does not panic or produce garbage output.
// The contract: negative values are treated as 0 (no overlap).
func TestSplitWithOptions_NegativeOverlapTreatedAsZero(t *testing.T) {
	text := loadTestData(t, "cbk_circular.txt")
	doc := makeDoc("cbk_circular.txt", models.TypeCBKCircular, text)

	baseline, _ := chunker.Split(doc)
	withNegative, err := chunker.SplitWithOptions(doc, chunker.Options{OverlapWords: -5})
	if err != nil {
		t.Fatalf("SplitWithOptions: unexpected error: %v", err)
	}
	if len(baseline) != len(withNegative) {
		t.Fatalf("negative OverlapWords changed chunk count: baseline=%d got=%d", len(baseline), len(withNegative))
	}
	for i := range baseline {
		if baseline[i].Text != withNegative[i].Text {
			t.Errorf("chunk %d: negative OverlapWords modified text", i)
		}
	}
}
