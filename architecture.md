# Hekima Architecture

> **Hekima** (Swahili: *wisdom*) is a domain-specific document chunking engine for East African AI systems.

---

## Problem Statement

Generic RAG chunking strategies destroy the structural meaning of East African documents:

- A CBK circular split at 500 characters loses its numbered section context
- A SACCO loan policy split at sentence boundaries separates penalty clauses from the grace period conditions they govern
- A court judgment split mid-ratio severs the legal reasoning from its conclusion

Hekima fixes this by **detecting document type first**, then applying the correct structural cutting grammar for that specific type.

---

## Design Principles

1. **Structure before size.** Chunks follow document grammar, not character limits.
2. **Domain-first detection.** The detector runs before any chunking decision is made.
3. **Fail loudly.** Unknown document types return an error rather than silently falling back to naive splitting.
4. **No external dependencies for core logic.** Detection and chunking use only the Go standard library.
5. **Composable outputs.** The CLI is a thin wrapper; the core packages are importable by any Go program.

---

## Package Layout

```
hekima/
├── cmd/hekima/         # CLI entrypoint — thin wrapper over internal packages
│   └── main.go
├── internal/
│   ├── detector/       # Document type detection
│   │   └── detector.go
│   └── chunker/        # Structure-aware chunking per document type
│       └── chunker.go
├── testdata/           # Realistic East African test documents
│   ├── cbk_circular.txt
│   ├── sacco_policy.txt
│   └── court_judgment.txt
└── docs/
    └── architecture.md # This file
```

---

## Data Flow

```
Input file (path)
      │
      ▼
┌─────────────┐
│  Detector   │  Reads raw text → identifies document type
│             │  Returns: DocType (enum), confidence signal, error
└──────┬──────┘
       │  DocType
       ▼
┌─────────────┐
│   Chunker   │  Applies structural grammar for that DocType
│             │  Returns: []Chunk{Text, Metadata}
└──────┬──────┘
       │  []Chunk
       ▼
┌─────────────┐
│     CLI     │  Formats output: human-readable | JSON stdout | JSON file
└─────────────┘
```

---

## Supported Document Types

| DocType | Example | Structural Grammar |
|---|---|---|
| `CBKCircular` | CBK/PG/01/2023 | Numbered sections, regulatory directives, effective dates |
| `SACCOPolicy` | Loan policy bylaws | Clause hierarchies, definitions, penalty/grace linkages |
| `CourtJudgment` | High Court, Milimani | Parties → Facts → Issues → Ratio → Orders |
| `LandTitle` | Certificate of Title | Parcel numbers, encumbrances, proprietor blocks |

---

## Detector

**File:** `internal/detector/detector.go`

The detector reads the raw document text and returns a `DocType` constant. Detection logic uses lexical signals — characteristic phrases, numbering patterns, and header formats unique to each document class.

Detection is **deterministic and stateless**: the same input always produces the same output. There is no ML model. This is intentional — East African regulatory documents have stable structural conventions that do not require probabilistic classification.

### DocType constants

```go
const (
    DocTypeCBKCircular  DocType = "cbk_circular"
    DocTypeSACCOPolicy  DocType = "sacco_policy"
    DocTypeCourtJudgment DocType = "court_judgment"
    DocTypeLandTitle    DocType = "land_title"
    DocTypeUnknown      DocType = "unknown"
)
```

---

## Chunker

**File:** `internal/chunker/chunker.go`

The chunker receives `(text string, docType DocType)` and returns `([]Chunk, error)`.

Each `Chunk` carries:

```go
type Chunk struct {
    Index    int               // Position in document
    Text     string            // Chunk content
    Metadata map[string]string // e.g. section number, clause title, document type
}
```

Chunking logic per type:

- **CBKCircular** — splits on numbered section headers (`1.`, `1.1`, `PART I`, etc.)
- **SACCOPolicy** — splits on clause boundaries; keeps penalty clause adjacent to its governing grace period
- **CourtJudgment** — splits on structural phases (parties, background, issues, ratio decidendi, orders)
- **LandTitle** — splits on block boundaries (proprietor, encumbrances, parcel description)

---

## CLI

**File:** `cmd/hekima/main.go`

```
Usage:
  hekima [flags] <input-file>

Flags:
  --output    Output mode: human (default), json-stdout, json-file
  --out-file  Path for JSON file output (required when --output=json-file)
```

### Output modes

| Mode | Description |
|---|---|
| `human` | Numbered chunks printed to stdout with metadata headers |
| `json-stdout` | JSON array of Chunk objects printed to stdout |
| `json-file` | JSON array written to a file at `--out-file` path |

---

## Roadmap

| Priority | Feature | Status |
|---|---|---|
| P0 | Document type detection | ✅ Done |
| P0 | Structure-aware chunking | ✅ Done |
| P0 | CLI with three output modes | ✅ Done |
| P0 | Architecture documentation | ✅ Done |
| P1 | PDF input support | 🔲 Next |
| P1 | Embedding-ready output (token counts, overlap control) | 🔲 Planned |
| P2 | HTTP server mode for pipeline integration | 🔲 Planned |
| P2 | Additional document types (NTSA forms, KRA notices) | 🔲 Planned |
| P3 | WASM build for browser-side chunking | 🔲 Planned |

---

## Adding a New Document Type

1. Add a new `DocType` constant in `detector/detector.go`
2. Add detection signals in the `Detect()` function
3. Add a chunking function in `chunker/chunker.go` and wire it into the `Chunk()` switch
4. Add a realistic test document in `testdata/`
5. Update the supported types table in this file

---

## Why Go

- Single binary deployment — drop it into any pipeline with no runtime dependency
- Strong standard library for text processing
- Straightforward concurrency for future batch processing of document corpora
- Easy to cross-compile for Linux servers common in East African cloud deployments