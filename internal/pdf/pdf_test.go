package pdf_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/the-veez/hekima/internal/pdf"
)

// pdfTestData returns the absolute path to a file in testdata/.
// PDF tests depend on real documents — synthetic PDFs would not exercise
// the layout and encoding edge cases that matter for East African documents.
func pdfTestData(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", filename)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("testdata file %q not found — skipping: %v", path, err)
	}
	return path
}

// TestExtractText_RealCBKDocument is the primary integration test.
// It runs pdftotext against the real CBK Act and asserts structural
// properties of the extracted text — not exact content, which would
// make the test brittle to minor PDF re-exports.
func TestExtractText_RealCBKDocument(t *testing.T) {
	path := pdfTestData(t, "cbk_act_cap491.pdf")

	text, err := pdf.ExtractText(path)
	if err != nil {
		t.Fatalf("ExtractText(%q): unexpected error: %v", path, err)
	}

	if strings.TrimSpace(text) == "" {
		t.Fatal("ExtractText: returned empty text from a known non-empty PDF")
	}

	// Assert structural signals present in the CBK Act.
	// These are document facts, not implementation details — they will
	// remain true across re-exports of the same document.
	requiredPhrases := []struct {
		phrase  string
		context string
	}{
		{"Central Bank of Kenya", "document identity"},
		{"Cap. 491", "document reference number"},
		{"Governor", "key role referenced throughout"},
		{"Banking Act", "primary referenced statute"},
		{"Cabinet Secretary", "government role referenced throughout"},
	}

	for _, rp := range requiredPhrases {
		if !strings.Contains(text, rp.phrase) {
			t.Errorf("ExtractText: expected phrase %q (%s) not found in extracted text", rp.phrase, rp.context)
		}
	}
}

// TestExtractText_OutputIsUTF8Clean verifies that the -enc UTF-8 flag
// is producing clean output. East African documents contain ligatures
// (ﬁ, ﬂ) and proper names with diacritics — these must not be mangled.
func TestExtractText_OutputIsUTF8Clean(t *testing.T) {
	path := pdfTestData(t, "cbk_act_cap491.pdf")

	text, err := pdf.ExtractText(path)
	if err != nil {
		t.Fatalf("ExtractText(%q): unexpected error: %v", path, err)
	}

	// Verify the output is valid UTF-8.
	// Invalid UTF-8 in the extracted text would corrupt every downstream
	// chunk and silently produce bad embeddings in the RAG pipeline.
	for i, r := range text {
		if r == '\uFFFD' {
			// '\uFFFD' is the Unicode replacement character — it appears
			// when a byte sequence is not valid UTF-8.
			t.Errorf("ExtractText: invalid UTF-8 sequence at byte offset %d — output contains replacement character", i)
			break
		}
	}
}

// TestExtractText_TextLengthIsPlausible guards against pdftotext
// silently returning a tiny fragment of a large document.
// The CBK Act is 34 pages — extracted text must be substantial.
func TestExtractText_TextLengthIsPlausible(t *testing.T) {
	path := pdfTestData(t, "cbk_act_cap491.pdf")

	text, err := pdf.ExtractText(path)
	if err != nil {
		t.Fatalf("ExtractText(%q): unexpected error: %v", path, err)
	}

	const minExpectedBytes = 10_000 // conservative floor for a 34-page Act
	if len(text) < minExpectedBytes {
		t.Errorf(
			"ExtractText: returned %d bytes, expected at least %d — pdftotext may have partially failed",
			len(text), minExpectedBytes,
		)
	}
}

// TestExtractText_FileNotFound verifies the correct error is returned
// when the input path does not exist.
func TestExtractText_FileNotFound(t *testing.T) {
	_, err := pdf.ExtractText("/tmp/hekima_does_not_exist_abc123.pdf")
	if err == nil {
		t.Fatal("ExtractText on missing file: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("ExtractText on missing file: error should mention 'not found', got: %v", err)
	}
}

// TestExtractText_EmptyFile verifies that a zero-byte file returns
// ErrEmptyFile rather than a pdftotext invocation that would fail
// with a confusing error message.
func TestExtractText_EmptyFile(t *testing.T) {
	// Create a temporary zero-byte file.
	tmp, err := os.CreateTemp("", "hekima_empty_*.pdf")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	_, err = pdf.ExtractText(tmp.Name())
	if err == nil {
		t.Fatal("ExtractText on empty file: expected error, got nil")
	}
	if !errors.Is(err, pdf.ErrEmptyFile) {
		t.Errorf("ExtractText on empty file: expected ErrEmptyFile, got: %v", err)
	}
}

// TestExtractText_NotAPDF verifies that passing a non-PDF file returns
// an error from pdftotext rather than garbage text.
func TestExtractText_NotAPDF(t *testing.T) {
	tmp, err := os.CreateTemp("", "hekima_notapdf_*.pdf")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())

	// Write plain text content with a .pdf extension.
	if _, err := tmp.WriteString("this is not a pdf file, just plain text masquerading as one"); err != nil {
		t.Fatalf("could not write to temp file: %v", err)
	}
	tmp.Close()

	_, err = pdf.ExtractText(tmp.Name())
	if err == nil {
		t.Fatal("ExtractText on non-PDF file: expected error, got nil")
	}
}
