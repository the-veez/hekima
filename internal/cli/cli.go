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
	"io"
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

// Run is the single entry point called by main. It wires RunWithIO to
// the process's real stdout and stderr. main.go calls only this.
func Run(args []string) error {
	return RunWithIO(args, os.Stdout, os.Stderr)
}

// RunWithIO is Run with the output streams injected, so tests can
// capture exactly what Hekima would print without touching the
// process-global os.Stdout/os.Stderr. Behaviour is otherwise
// identical to Run — this is the function Run delegates to.
func RunWithIO(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("hekima", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr) }

	jsonOut := fs.Bool("json", false, "Output chunks as JSON array to stdout")
	outputFile := fs.String("output", "", "Write JSON chunks to this file path")

	// stdlib flag.Parse stops scanning for flags at the first positional
	// argument — "hekima file.pdf --json" would silently leave --json
	// unparsed and fall through to human-readable output. reorderArgs
	// partitions flags and positionals so order never matters.
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		printUsage(stderr)
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
		fmt.Fprintf(stdout, "✓ Wrote %d chunks to %s\n", len(chunks), *outputFile)

	case *jsonOut:
		return printJSON(stdout, chunks)

	default:
		printHuman(stdout, doc, chunks)
	}

	return nil
}

// knownValueFlags lists flags that consume the following argument as
// their value (e.g. "--output out.json"). Boolean flags like --json
// do not consume a following argument and are omitted here.
var knownValueFlags = map[string]bool{
	"-output":  true,
	"--output": true,
}

// reorderArgs partitions args into [flags..., positionals...] so that
// flag.FlagSet.Parse sees every flag regardless of where the caller
// placed it on the command line.
//
// stdlib flag.Parse stops looking for flags the moment it encounters
// the first argument that doesn't start with "-": everything after
// that point, flags included, is treated as a positional argument.
// "hekima file.pdf --json" would therefore silently run in
// human-readable mode instead of JSON mode — a sharp edge for a tool
// whose primary consumers are scripts and pipelines. Reordering here
// means flag position never matters.
func reorderArgs(args []string) []string {
	var flags, positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// If this flag takes a value and it wasn't passed as
			// --flag=value, the next token is that value, not a
			// positional argument — consume it together.
			if knownValueFlags[arg] && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	return append(flags, positionals...)
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
func printHuman(w io.Writer, doc models.Document, chunks []models.Chunk) {
	fmt.Fprintf(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(w, "  HEKIMA — Document Analysis\n")
	fmt.Fprintf(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(w, "  File     : %s\n", doc.Filename)
	fmt.Fprintf(w, "  Type     : %s\n", doc.Type)
	fmt.Fprintf(w, "  Chunks   : %d\n", len(chunks))
	fmt.Fprintf(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, chunk := range chunks {
		fmt.Fprintf(w, "[ Chunk %d | Section: %s ]\n", chunk.ID, chunk.Section)
		fmt.Fprintf(w, "%s\n", chunk.Text)
		fmt.Fprintf(w, "\n──────────────────────────────────────────\n\n")
	}
}

func printJSON(w io.Writer, chunks []models.Chunk) error {
	out, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON serialization failed: %w", err)
	}
	fmt.Fprintln(w, string(out))
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

func printUsage(w io.Writer) {
	fmt.Fprint(w, `
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
