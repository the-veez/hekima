package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/the-veez/hekima/internal/models"
)

// This file is an internal test (package cli, not cli_test) because it
// exercises reorderArgs directly — a private function with no exported
// equivalent worth adding just for testability.

// loadTestData reads a file from the shared testdata directory at the
// repo root. From internal/cli/, the repo root is ../../.
func loadTestData(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadTestData: cannot read %s: %v", path, err)
	}
	return string(data)
}

// run is a test helper that calls RunWithIO and returns the captured
// stdout, stderr, and error, so each test only has to assert on what
// it actually cares about.
func run(args []string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	err = RunWithIO(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), err
}

func TestRun_NoInputFile(t *testing.T) {
	_, _, err := run([]string{})
	if err == nil {
		t.Fatal("Run with no args: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no input file") {
		t.Errorf("Run with no args: error = %q, want it to mention \"no input file\"", err.Error())
	}
}

func TestRun_MissingFile(t *testing.T) {
	_, _, err := run([]string{"does-not-exist.txt"})
	if err == nil {
		t.Fatal("Run with missing file: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot access file") {
		t.Errorf("Run with missing file: error = %q, want it to mention \"cannot access file\"", err.Error())
	}
}

func TestRun_OversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")

	// maxInputBytes is 10MB; write one byte over the limit.
	oversized := make([]byte, maxInputBytes+1)
	for i := range oversized {
		oversized[i] = 'a'
	}
	if err := os.WriteFile(path, oversized, 0600); err != nil {
		t.Fatalf("setup: cannot write oversized test file: %v", err)
	}

	_, _, err := run([]string{path})
	if err == nil {
		t.Fatal("Run with oversized file: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeding") {
		t.Errorf("Run with oversized file: error = %q, want it to mention the size limit", err.Error())
	}
}

func TestRun_UnknownDocumentType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "generic.txt")

	text := "This is a generic memo with no structural markers Hekima recognizes."
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatalf("setup: cannot write test file: %v", err)
	}

	_, _, err := run([]string{path})
	if err == nil {
		t.Fatal("Run with unrecognised document: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "could not identify document type") {
		t.Errorf("Run with unrecognised document: error = %q, want it to mention identification failure", err.Error())
	}
}

func TestRun_HumanReadableOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sacco_policy.txt")
	text := loadTestData(t, "sacco_policy.txt")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatalf("setup: cannot write test file: %v", err)
	}

	stdout, _, err := run([]string{path})
	if err != nil {
		t.Fatalf("Run(sacco_policy): unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "HEKIMA") {
		t.Error("Run(sacco_policy): human-readable output missing the Hekima banner")
	}
	if !strings.Contains(stdout, "sacco_policy") {
		t.Error("Run(sacco_policy): human-readable output missing the filename")
	}
	if !strings.Contains(stdout, string(models.TypeSACCOPolicy)) {
		t.Errorf("Run(sacco_policy): human-readable output missing the detected type %q", models.TypeSACCOPolicy)
	}
	if !strings.Contains(stdout, "DEFAULT AND PENALTIES") {
		t.Error("Run(sacco_policy): human-readable output missing an expected section heading")
	}
}

func TestRun_JSONOutputToStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cbk_circular.txt")
	text := loadTestData(t, "cbk_circular.txt")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatalf("setup: cannot write test file: %v", err)
	}

	stdout, _, err := run([]string{path, "--json"})
	if err != nil {
		t.Fatalf("Run(cbk_circular, --json): unexpected error: %v", err)
	}

	var chunks []models.Chunk
	if jsonErr := json.Unmarshal([]byte(stdout), &chunks); jsonErr != nil {
		t.Fatalf("Run(cbk_circular, --json): stdout is not valid JSON: %v\nstdout was:\n%s", jsonErr, stdout)
	}
	if len(chunks) == 0 {
		t.Error("Run(cbk_circular, --json): produced zero chunks")
	}
	for _, c := range chunks {
		if c.DocType != string(models.TypeCBKCircular) {
			t.Errorf("chunk %d: DocType = %q, want %q", c.ID, c.DocType, models.TypeCBKCircular)
		}
	}
}

// TestRun_JSONFlagOrderDoesNotMatter is the regression test for the
// flag-ordering bug reorderArgs fixes: "hekima file.txt --json" must
// produce JSON on stdout, not silently fall through to human-readable
// output. Before the fix, stdlib flag.Parse would treat --json as a
// second positional argument once the filename came first.
func TestRun_JSONFlagOrderDoesNotMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cbk_circular.txt")
	text := loadTestData(t, "cbk_circular.txt")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatalf("setup: cannot write test file: %v", err)
	}

	// Flag placed AFTER the positional filename — the exact ordering
	// that previously broke.
	stdout, _, err := run([]string{path, "--json"})
	if err != nil {
		t.Fatalf("Run(file, --json): unexpected error: %v", err)
	}

	var chunks []models.Chunk
	if jsonErr := json.Unmarshal([]byte(stdout), &chunks); jsonErr != nil {
		t.Fatalf(
			"Run(file, --json): stdout is not valid JSON when --json comes after the filename — "+
				"the flag-ordering bug has regressed. Error: %v\nstdout was:\n%s",
			jsonErr, stdout,
		)
	}
}

func TestRun_OutputFileIsWrittenWithCorrectPermissions(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "sample_act.txt")
	outputPath := filepath.Join(dir, "chunks.json")

	text := loadTestData(t, "sample_act.txt")
	if err := os.WriteFile(inputPath, []byte(text), 0600); err != nil {
		t.Fatalf("setup: cannot write test file: %v", err)
	}

	stdout, _, err := run([]string{inputPath, "--output", outputPath})
	if err != nil {
		t.Fatalf("Run(sample_act, --output): unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "Wrote") {
		t.Errorf("Run(sample_act, --output): stdout = %q, want a confirmation message", stdout)
	}

	info, statErr := os.Stat(outputPath)
	if statErr != nil {
		t.Fatalf("Run(sample_act, --output): output file was not created: %v", statErr)
	}

	// 0600: owner read/write only. Chunk data may contain sensitive
	// financial or legal content extracted from the source document.
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("Run(sample_act, --output): output file has permissions %o, want 0600", perm)
	}

	written, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("Run(sample_act, --output): cannot read output file: %v", readErr)
	}
	var chunks []models.Chunk
	if jsonErr := json.Unmarshal(written, &chunks); jsonErr != nil {
		t.Fatalf("Run(sample_act, --output): output file is not valid JSON: %v", jsonErr)
	}
	if len(chunks) == 0 {
		t.Error("Run(sample_act, --output): output file contains zero chunks")
	}
}

// TestRun_OutputFlagBeforeFilename verifies --output (a value-taking
// flag) is reordered correctly when given before the positional
// filename, including consuming its value rather than mistaking the
// value for a second positional argument.
func TestRun_OutputFlagBeforeFilename(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "sample_act.txt")
	outputPath := filepath.Join(dir, "chunks.json")

	text := loadTestData(t, "sample_act.txt")
	if err := os.WriteFile(inputPath, []byte(text), 0600); err != nil {
		t.Fatalf("setup: cannot write test file: %v", err)
	}

	_, _, err := run([]string{"--output", outputPath, inputPath})
	if err != nil {
		t.Fatalf("Run(--output, path, file): unexpected error: %v", err)
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		t.Fatalf("Run(--output, path, file): output file was not created: %v", statErr)
	}
}

func TestReorderArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "filename then boolean flag",
			in:   []string{"file.pdf", "--json"},
			want: []string{"--json", "file.pdf"},
		},
		{
			name: "boolean flag then filename (already correct order)",
			in:   []string{"--json", "file.pdf"},
			want: []string{"--json", "file.pdf"},
		},
		{
			name: "filename then value flag and its value",
			in:   []string{"file.pdf", "--output", "out.json"},
			want: []string{"--output", "out.json", "file.pdf"},
		},
		{
			name: "value flag with = syntax after filename",
			in:   []string{"file.pdf", "--output=out.json"},
			want: []string{"--output=out.json", "file.pdf"},
		},
		{
			name: "no flags at all",
			in:   []string{"file.pdf"},
			want: []string{"file.pdf"},
		},
		{
			name: "empty args",
			in:   []string{},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderArgs(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("reorderArgs(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("reorderArgs(%v) = %v, want %v", tt.in, got, tt.want)
					break
				}
			}
		})
	}
}
