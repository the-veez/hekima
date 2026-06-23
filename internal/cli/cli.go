// Package cli handles all command-line concerns for Hekima: flag parsing,
// input validation, orchestration of the detect→chunk pipeline, and
// output formatting.
//
// main.go calls cli.Run() and nothing else. All logic lives here.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/the-veez/hekima/internal/chunker"
	"github.com/the-veez/hekima/internal/detector"
	"github.com/the-veez/hekima/internal/models"
	"github.com/the-veez/hekima/internal/pdf"
)

// maxInputBytes is the maximum file size Hekima will read into memory.
// Documents larger than this are corpora or concatenated inputs that
// must be pre-split before passing to Hekima.
const maxInputBytes = 10 * 1024 * 1024 // 10 MB

// Run is the single entry point called by main. It parses flags,
// validates input, runs the pipeline, and writes output.
// All errors are returned to main — nothing calls os.Exit from here.
func Run(args []string) error {
	fs := flag.NewFlagSet("hekima", flag.ContinueOnError)
	fs.Usage = printUsage

	jsonOut := fs.Bool("json", false, "Output chunks as JSON array to stdout")
	outputFile := fs.String("output", "", "Write JSON chunks to this file path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		printUsage()
		return fmt.Errorf("no input file specified")
	}

	filepath := fs.Arg(0)

	content, err := readFile(filepath)
	if err != nil {
		return err
	}

	// Step 1: Detect document type.
	doc := detector.Detect(filepath, string(content))

	// Step 2: Chunk. Surface detection failures explicitly.
	chunks, err := chunker.Split(doc)
	if err != nil {
		if errors.Is(err, chunker.ErrUnknownDocType) {
			return fmt.Errorf(
				"could not identify document type for %q — "+
					"Hekima supports: cbk_circular, sacco_policy, court_judgment, land_title. "+
					"Verify the document contains the expected structural markers",
				filepath,
			)
		}
		return fmt.Errorf("chunking failed: %w", err)
	}

	if len(chunks) == 0 {
		return fmt.Errorf("document %q produced zero chunks: file may be empty or contain only whitespace", filepath)
	}

	// Step 3: Output.
	switch {
	case *outputFile != "":
		if err := writeJSON(*outputFile, chunks); err != nil {
			return fmt.Errorf("cannot write output file: %w", err)
		}
		fmt.Printf("✓ Wrote %d chunks to %s\n", len(chunks), *outputFile)

	case *jsonOut:
		return printJSON(chunks)

	default:
		printHuman(doc, chunks)
	}

	return nil
}

// readFile enforces the size limit and reads the file content.
// readFile reads a document into memory, routing PDF files through the
// text extraction pipeline before returning raw text.
// Enforces the size limit for plain text files; PDF files have their
// own limit enforced inside the pdf package.
func readFile(filepath string) ([]byte, error) {
	if strings.HasSuffix(strings.ToLower(filepath), ".pdf") {
		text, err := pdf.ExtractText(filepath)
		if err != nil {
			return nil, fmt.Errorf("PDF extraction failed: %w", err)
		}
		return []byte(text), nil
	}

	info, err := os.Stat(filepath)
	if err != nil {
		return nil, fmt.Errorf("cannot access file %q: %w", filepath, err)
	}
	if info.Size() > maxInputBytes {
		return nil, fmt.Errorf(
			"file %q is %d bytes, exceeding the %d byte limit: pre-split large documents before passing to hekima",
			filepath, info.Size(), maxInputBytes,
		)
	}

	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("cannot read file %q: %w", filepath, err)
	}
	return content, nil
}
func printHuman(doc models.Document, chunks []models.Chunk) {
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  HEKIMA — Document Analysis\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  File     : %s\n", doc.Filename)
	fmt.Printf("  Type     : %s\n", doc.Type)
	fmt.Printf("  Chunks   : %d\n", len(chunks))
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, chunk := range chunks {
		fmt.Printf("[ Chunk %d | Section: %s ]\n", chunk.ID, chunk.Section)
		fmt.Printf("%s\n", chunk.Text)
		fmt.Printf("\n──────────────────────────────────────────\n\n")
	}
}

func printJSON(chunks []models.Chunk) error {
	out, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON serialization failed: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func writeJSON(filename string, chunks []models.Chunk) error {
	out, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return err
	}
	// 0600: owner read/write only. Chunk data may contain sensitive
	// financial or legal content from the source documents.
	return os.WriteFile(filename, out, 0600)
}

func printUsage() {
	fmt.Fprint(os.Stderr, `
Hekima — Domain-specific document chunker for East African AI systems

Usage:
  hekima <document>                      Human-readable chunk output
  hekima <document> --json               JSON array to stdout
  hekima <document> --output out.json    Write JSON to file

Supported document types:
  cbk_circular     Central Bank of Kenya circulars
  sacco_policy     SACCO loan and operational policies
  court_judgment   Kenyan court judgments and rulings
  land_title       Land registration documents

Examples:
  hekima testdata/sacco_policy.txt
  hekima testdata/cbk_circular.txt --json
  hekima testdata/court_judgment.txt --output chunks.json

`)
}
