// Package pdf extracts plain text from PDF documents using pdftotext,
// part of the poppler-utils suite.
//
// Hekima uses pdftotext rather than a pure-Go PDF library because
// real East African government documents — CBK circulars, court judgments,
// land titles — frequently have complex layouts, headers, footers, and
// column structures that pure-Go libraries mangle. pdftotext has been
// battle-tested against these layouts for two decades.
//
// System requirement: poppler-utils must be installed.
//
//	Ubuntu/Debian: sudo apt install poppler-utils
//	macOS:         brew install poppler
//
// If pdftotext is not installed, ExtractText returns ErrPopplerNotFound
// with installation instructions. It never silently falls back.
package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrPopplerNotFound is returned when pdftotext is not installed.
// It is a typed sentinel so callers can present installation instructions
// rather than a generic "command not found" message.
var ErrPopplerNotFound = errors.New("pdftotext not found: install poppler-utils (Ubuntu: sudo apt install poppler-utils / macOS: brew install poppler)")

// ErrNoTextLayer is returned when pdftotext runs successfully but
// produces no output. This indicates a pure image scan with no embedded
// text layer. OCR is required and is out of scope for this version.
var ErrNoTextLayer = errors.New("PDF contains no extractable text layer: document may be a pure image scan; OCR support is not available in this version")

// ErrEmptyFile is returned when the PDF file exists but has zero bytes.
var ErrEmptyFile = errors.New("PDF file is empty")

// maxPDFBytes is the maximum PDF file size Hekima will process.
// PDFs larger than this are either entire document corpora or contain
// embedded assets (images, attachments) that inflate the size far beyond
// what a single policy or regulatory document should be.
const maxPDFBytes = 50 * 1024 * 1024 // 50 MB

// ExtractText extracts the plain text content from a PDF file using
// pdftotext. The returned string is ready to pass directly to
// detector.Detect() and chunker.Split().
//
// Layout mode (-layout) is used to preserve column and table structure
// as closely as possible in the plain text output, which improves
// section boundary detection on regulatory documents.
//
// Returns a typed error for each failure mode so callers can give the
// user an actionable message rather than a raw error string.
func ExtractText(filepath string) (string, error) {
	if err := validateFile(filepath); err != nil {
		return "", err
	}

	if err := checkPopplerInstalled(); err != nil {
		return "", err
	}

	text, err := runPDFToText(filepath)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%w: file=%q", ErrNoTextLayer, filepath)
	}

	return text, nil
}

// validateFile checks that the file exists, is readable, and is within
// the size limit before we invoke pdftotext.
func validateFile(filepath string) error {
	info, err := os.Stat(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("PDF file not found: %q", filepath)
		}
		return fmt.Errorf("cannot access PDF file %q: %w", filepath, err)
	}

	if info.Size() == 0 {
		return fmt.Errorf("%w: %q", ErrEmptyFile, filepath)
	}

	if info.Size() > maxPDFBytes {
		return fmt.Errorf(
			"PDF file %q is %d bytes, exceeding the %d byte limit: "+
				"split large PDFs before passing to hekima",
			filepath, info.Size(), maxPDFBytes,
		)
	}

	return nil
}

// checkPopplerInstalled verifies pdftotext is on PATH before attempting
// extraction. Returns ErrPopplerNotFound if it is absent so the caller
// can surface installation instructions to the user.
func checkPopplerInstalled() error {
	_, err := exec.LookPath("pdftotext")
	if err != nil {
		return ErrPopplerNotFound
	}
	return nil
}

// runPDFToText invokes pdftotext and returns the extracted text.
// stdout receives the text; stderr is captured separately so error
// messages from pdftotext are included in the returned error.
//
// Flags:
//
//	-layout   Preserve original physical layout — important for documents
//	          with numbered sections, tables, and multi-column layouts
//	-enc UTF-8 Force UTF-8 output — East African documents may contain
//	          Swahili characters and proper names with diacritics
//	-         Write output to stdout instead of a file
func runPDFToText(filepath string) (string, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", filepath, "-")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// pdftotext writes diagnostics to stderr — include them in the
		// error so the user sees what actually went wrong.
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("pdftotext failed on %q: %s", filepath, errMsg)
	}

	return stdout.String(), nil
}
