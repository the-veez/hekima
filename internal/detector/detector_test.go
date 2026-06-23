package detector_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/the-veez/hekima/internal/detector"
	"github.com/the-veez/hekima/internal/models"
)

// loadTestData reads a file from the shared testdata directory at the
// repo root. Tests must not embed document text inline — real documents
// live in testdata/ so detection is tested against authentic content.
func loadTestData(t *testing.T, filename string) string {
	t.Helper()
	// From internal/detector/, the repo root is ../../
	path := filepath.Join("..", "..", "testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadTestData: cannot read %s: %v", path, err)
	}
	return string(data)
}

func TestDetect_KnownDocumentTypes(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		expectedType models.DocumentType
	}{
		{
			name:         "CBK circular is detected correctly",
			file:         "cbk_circular.txt",
			expectedType: models.TypeCBKCircular,
		},
		{
			name:         "SACCO policy is detected correctly",
			file:         "sacco_policy.txt",
			expectedType: models.TypeSACCOPolicy,
		},
		{
			name:         "court judgment is detected correctly",
			file:         "court_judgment.txt",
			expectedType: models.TypeCourtJudgment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := loadTestData(t, tt.file)
			doc := detector.Detect(tt.file, text)

			if doc.Type != tt.expectedType {
				t.Errorf("Detect(%q): got type %q, want %q", tt.file, doc.Type, tt.expectedType)
			}
			if doc.Filename != tt.file {
				t.Errorf("Detect(%q): got filename %q, want %q", tt.file, doc.Filename, tt.file)
			}
			if doc.RawText != text {
				t.Errorf("Detect(%q): RawText was not preserved", tt.file)
			}
		})
	}
}

func TestDetect_EmptyDocument(t *testing.T) {
	doc := detector.Detect("empty.txt", "")
	if doc.Type != models.TypeUnknown {
		t.Errorf("empty document: got type %q, want %q", doc.Type, models.TypeUnknown)
	}
}

func TestDetect_UnrecognisedDocument(t *testing.T) {
	text := "This is a generic document with no structural markers whatsoever."
	doc := detector.Detect("random.txt", text)
	if doc.Type != models.TypeUnknown {
		t.Errorf("unrecognised document: got type %q, want %q", doc.Type, models.TypeUnknown)
	}
}

func TestDetect_SingleSignatureIsNotEnough(t *testing.T) {
	// Only one CBK signature present — must not be classified as CBK circular.
	text := "The Governor has issued a statement."
	doc := detector.Detect("ambiguous.txt", text)
	if doc.Type != models.TypeUnknown {
		t.Errorf("single signature: got type %q, want %q — minimum threshold of 2 not enforced", doc.Type, models.TypeUnknown)
	}
}

func TestDetect_FilenameIsPreserved(t *testing.T) {
	doc := detector.Detect("my_document.txt", "some text")
	if doc.Filename != "my_document.txt" {
		t.Errorf("filename not preserved: got %q, want %q", doc.Filename, "my_document.txt")
	}
}

// TestDetect_Legislation verifies that Kenyan Acts and Statutes are
// identified as TypeLegislation, and — critically — not misclassified
// as TypeCBKCircular due to shared vocabulary ("Central Bank of Kenya",
// "Banking Act" both cite the Act and the circular that implements it).
func TestDetect_Legislation(t *testing.T) {
	text := loadTestData(t, "sample_act.txt")
	doc := detector.Detect("sample_act.txt", text)

	if doc.Type != models.TypeLegislation {
		t.Errorf("Detect(sample_act.txt): got type %q, want %q", doc.Type, models.TypeLegislation)
	}
	if doc.Filename != "sample_act.txt" {
		t.Errorf("Detect(sample_act.txt): got filename %q, want %q", doc.Filename, "sample_act.txt")
	}
	if doc.RawText != text {
		t.Error("Detect(sample_act.txt): RawText was not preserved")
	}
}

// TestDetect_LegislationNotConfusedWithCBKCircular verifies the actual
// CBK Act PDF — which shares "Central Bank of Kenya" and "Banking Act"
// vocabulary with regulatory circulars — is still correctly identified
// as TypeLegislation. This is the precision problem identified in the
// PDF extraction work: a circular cites the Act it implements, but
// only the Act itself contains statutory structural markers like
// "Cap.", "Short title", and "An Act of Parliament".
func TestDetect_LegislationNotConfusedWithCBKCircular(t *testing.T) {
	text := loadTestData(t, "cbk_act_cap491.txt")
	doc := detector.Detect("cbk_act_cap491.txt", text)

	if doc.Type != models.TypeLegislation {
		t.Errorf("Detect(cbk_act_cap491.txt): got type %q, want %q — the Act was misclassified as a circular or left unknown", doc.Type, models.TypeLegislation)
	}
}
