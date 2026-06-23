# Hekima

> *Hekima* (Swahili) — wisdom, intelligence

**Domain-specific document chunking for East African AI systems.**

---

## The Problem

AI document systems (RAG pipelines) retrieve answers by splitting documents into chunks and searching them. Every mainstream tool — LangChain, LlamaIndex, Unstructured — uses generic splitting strategies: fixed character counts, sentence boundaries, or semantic similarity scores.

These strategies destroy the structure of East African documents.

A CBK circular cut at 500 characters loses the numbered section context. A SACCO loan policy split at sentence boundaries separates penalty clauses from the grace period conditions they govern. A Kenyan court judgment chunked generically fragments the ORDER from the FINDINGS that justify it.

**The result:** AI systems built on these documents give wrong, incomplete, or hallucinated answers — even when the correct information is in the document.

---

## What Hekima Does

Hekima is a document chunking engine that understands the *structure* of East African document types before it cuts them.

Input: Raw document (PDF, text)

↓

[ Document Type Detector ]

Identifies: CBK Circular, SACCO Policy,

Court Judgment, Land Title...

↓

[ Structure-Aware Chunker ]

Applies the correct cutting grammar

for that document type

↓

Output: Semantically complete chunks

with rich metadata

Each output chunk carries:
- The section it belongs to (e.g. "Default and Penalties")
- The document type
- Its position in the document
- The source filename

---

## Supported Document Types

| Type | Description | Cutting Strategy |
|------|-------------|-----------------|
| `cbk_circular` | Central Bank of Kenya circulars | Numbered section boundaries |
| `sacco_policy` | SACCO loan and operational policies | Heading-based sections |
| `court_judgment` | Kenyan court judgments and rulings | Legal structural markers |
| `land_title` | Land registration documents | Clause boundaries |
| `unknown` | Unrecognized documents | Paragraph-based fallback |

---

## Getting Started

### Prerequisites
- Go 1.21 or higher

### Installation

```bash
git clone https://github.com/the-veez/hekima.git
cd hekima
go mod tidy
```

### Usage

```bash
go run ./cmd/hekima/main.go testdata/sacco_policy.txt
go run ./cmd/hekima/main.go testdata/cbk_circular.txt --json
go run ./cmd/hekima/main.go testdata/court_judgment.txt --output chunks.json
```

---

## Project Structure

hekima/

├── cmd/hekima/main.go          # CLI entry point

├── internal/

│   ├── detector/detector.go    # Document type detection

│   └── chunker/chunker.go      # Structure-aware chunking

├── testdata/                   # Sample East African documents

├── docs/architecture.md        # Technical architecture notes

└── README.md

---

## Why Go

Single binary, no runtime dependencies, fast text processing, built-in concurrency. Deployable anywhere in Africa where infrastructure is constrained.

---

## Roadmap

- [x] Document type detection
- [x] Structure-aware chunking (SACCO, CBK, Court)
- [ ] PDF input support
- [ ] REST API endpoint
- [ ] Swahili document support
- [ ] Evaluation suite

---

*Built in Nairobi. Solving African problems with African context.*