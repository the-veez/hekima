// Hekima CLI — domain-specific document chunker for East African documents.
//
// Usage:
//
//	hekima <document.txt>                 Print chunks to stdout
//	hekima <document.txt> --json          Output as JSON array to stdout
//	hekima <document.txt> --output file   Write JSON chunks to file
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/the-veez/hekima/internal/chunker"
	"github.com/the-veez/hekima/internal/detector"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	filepath := os.Args[1]
	flags := parseFlags(os.Args[2:])

	// Read the document
	content, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Step 1: Detect document type
	doc := detector.Detect(filepath, string(content))

	// Step 2: Chunk intelligently
	chunks := chunker.Split(doc)

	// Step 3: Output
	if outputFile, ok := flags["output"]; ok {
		if err := writeJSON(outputFile, chunks); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Wrote %d chunks to %s\n", len(chunks), outputFile)
		return
	}

	if _, ok := flags["json"]; ok {
		printJSON(chunks)
		return
	}

	// Default: human-readable output
	printHuman(doc, chunks)
}

// printHuman prints a readable summary useful during development
// to quickly see whether chunking is working correctly.
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

// printJSON outputs chunks as a JSON array to stdout.
func printJSON(chunks []chunker.Chunk) {
	out, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}

// writeJSON writes chunks to a JSON file.
func writeJSON(filename string, chunks []chunker.Chunk) error {
	out, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, out, 0644)
}

// parseFlags parses simple --key or --key value flags from args.
func parseFlags(args []string) map[string]string {
	flags := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[key] = args[i+1]
				i++
			} else {
				flags[key] = "true"
			}
		}
	}
	return flags
}

func printUsage() {
	fmt.Println(`
Hekima — Domain-specific document chunker for East African AI systems

Usage:
  hekima <document.txt>                    Human-readable output
  hekima <document.txt> --json             JSON array to stdout
  hekima <document.txt> --output out.json  Write JSON to file

Supported document types:
  cbk_circular     Central Bank of Kenya circulars
  sacco_policy     SACCO loan and operational policies
  court_judgment   Kenyan court judgments and rulings
  unknown          Unrecognized (paragraph fallback)

Examples:
  hekima testdata/sacco_policy.txt
  hekima testdata/cbk_circular.txt --json
  hekima testdata/court_judgment.txt --output chunks.json
`)
}
