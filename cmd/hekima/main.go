// Hekima CLI — domain-specific document chunker for East African documents.
//
// Usage:
//
//	hekima <document.txt>              Print chunks to stdout
//	hekima <document.txt> --json       Output as JSON array to stdout
//	hekima <document.txt> --output f   Write JSON chunks to file f
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/the-veez/hekima/internal/chunker"
	"github.com/the-veez/hekima/internal/detector"
)

// maxInputBytes is the maximum file size Hekima will load into memory.
// Documents larger than this are almost certainly not single policy or
// regulatory files — they are corpora or concatenated inputs that should
// be pre-split before being passed to Hekima.
const maxInputBytes = 10 * 1024 * 1024 // 10 MB

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "hekima: %v\n", err)
		os.Exit(1)
	}
}

// run is the real entrypoint. Separating it from main() makes error
// handling clean and makes the CLI testable without os.Exit.
func run(args []string) error {
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

	// Enforce file size limit before reading.
	info, err := os.Stat(filepath)
	if err != nil {
		return fmt.Errorf("cannot access file %q: %w", filepath, err)
	}
	if info.Size() > maxInputBytes {
		return fmt.Errorf(
			"file %q is %d bytes, which exceeds the %d byte limit: pre-split large documents before passing to hekima",
			filepath, info.Size(), maxInputBytes,
		)
	}

	content, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("cannot read file %q: %w", filepath, err)
	}

	// Step 1: Detect document type.
	doc := detector.Detect(filepath, string(content))

	// Step 2: Chunk intelligently. Surface detection failures to the caller.
	chunks, err := chunker.Split(doc)
	if err != nil {
		if errors.Is(err, chunker.ErrUnknownDocType) {
			return fmt.Errorf(
				"could not identify document type for %q — "+
					"Hekima supports: cbk_circular, sacco_policy, court_judgment, land_title. "+
					"Check that the document is one of these types and contains the expected structural markers",
				filepath,
			)
		}
		return fmt.Errorf("chunking failed: %w", err)
	}

	if len(chunks) == 0 {
		return fmt.Errorf("document %q produced zero chunks: the file may be empty or contain only whitespace", filepath)
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

func printHuman(doc detector.Document, chunks []chunker.Chunk) {
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

func printJSON(chunks []chunker.Chunk) error {
	out, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON serialization failed: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func writeJSON(filename string, chunks []chunker.Chunk) error {
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
  hekima <document>                        Human-readable chunk output
  hekima <document> --json                 JSON array to stdout
  hekima <document> --output out.json      Write JSON to file

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
