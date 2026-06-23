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