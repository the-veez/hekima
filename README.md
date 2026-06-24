# Hekima

> *Hekima* (Swahili) — wisdom, intelligence

**Domain-specific document chunking engine for East African AI systems.**

---

## The Problem

AI retrieval systems (RAG pipelines) fail on East African documents because they use generic chunking strategies — fixed character counts or sentence boundaries — that destroy document structure.

A CBK circular split at 500 characters loses its numbered section context. A SACCO loan policy split at sentence boundaries separates penalty clauses from the grace period conditions they govern. A Kenyan court judgment chunked generically fragments the ORDER from the FINDINGS that justify it.

**The result:** AI systems built on these documents give wrong, incomplete, or hallucinated answers — even when the correct information is in the document.

---

## What Hekima Does

Hekima detects the document type first, then applies the correct structural cutting grammar for that specific type.

Each output chunk carries:
- `section` — the structural section it belongs to (e.g. "3. Customer Data Protection Requirements")
- `doc_type` — the detected document type
- `token_count` — estimated token count for embedding model context management
- `metadata` — document-type-specific fields (e.g. `part` for legislation)
- `filename` — source document

---

## Supported Document Types

| Type | Description | Boundary Signal |
|---|---|---|
| `cbk_circular` | Central Bank of Kenya circulars | Top-level numbered sections only — subsections stay inside parent |
| `sacco_policy` | SACCO loan and operational policies | ALL-CAPS heading lines |
| `court_judgment` | Kenyan court judgments and rulings | BACKGROUND, FACTS, ANALYSIS, FINDINGS, ORDER |
| `land_title` | Land registration documents | Blank line boundaries |
| `legislation` | Kenyan Acts and Statutes | Part headers + alphanumeric section numbers (4A, 33U, 51B) |

Detection is deterministic and stateless — no ML model. The same document always produces the same result.

---

## Quickstart — Docker (recommended)

No Go or poppler-utils installation required.

    docker build -t hekima .
    docker run -p 8080:8080 hekima

Then chunk a document:

    curl -X POST http://localhost:8080/chunk -F "file=@cbk_circular.pdf"

---

## HTTP API

### POST /chunk

Chunk a document. Accepts multipart/form-data.

Form fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `file` | file | Yes | Document to chunk (.txt or .pdf, max 10 MB) |
| `overlap_words` | integer | No | Words to repeat at chunk boundaries (default: 0) |

Response — 200 OK: JSON array of Chunk objects, each with id, text, section, doc_type, filename, token_count, metadata.

Error responses:

| Status | Meaning |
|---|---|
| 400 | Bad request — missing file field, invalid overlap_words, file too large |
| 405 | Wrong HTTP method |
| 422 | Document type could not be identified |
| 500 | Internal error |

Examples:

    # Basic chunking
    curl -X POST http://localhost:8080/chunk -F "file=@sacco_policy.txt"

    # With overlap for better RAG context continuity
    curl -X POST http://localhost:8080/chunk -F "file=@cbk_circular.pdf" -F "overlap_words=20"

    # PDF legislation
    curl -X POST http://localhost:8080/chunk -F "file=@cbk_act_cap491.pdf"

### GET /health

Liveness check for load balancers and uptime monitors.

    curl http://localhost:8080/health
    # {"status":"ok"}

---

## CLI Usage

If you have Go installed:

    go run ./cmd/hekima testdata/cbk_circular.txt
    go run ./cmd/hekima testdata/cbk_circular.txt --json
    go run ./cmd/hekima testdata/cbk_circular.txt --output chunks.json
    go run ./cmd/hekima testdata/cbk_act_cap491.pdf --json

Server mode:

    go run ./cmd/hekima --serve
    go run ./cmd/hekima --serve --port 9090

---

## Overlap and Token Counts

overlap_words repeats the last N words of each chunk at the start of the next. This preserves sentence context severed at a boundary. Recommended range: 10-30 words.

token_count on each chunk is estimated using a word count x 1.3 heuristic (approximates BPE token counts for English and Swahili prose). Use it to enforce context window limits when batching chunks for an embedding model.

---

## Building from Source

Prerequisites:
- Go 1.26 or higher
- poppler-utils for PDF support (sudo apt install poppler-utils)

    git clone https://github.com/the-veez/hekima.git
    cd hekima
    go mod tidy
    go build -o hekima ./cmd/hekima
    ./hekima testdata/cbk_circular.txt

Run tests:

    go test ./...

---

## Project Structure

    hekima/
    +-- cmd/hekima/main.go       entry point: CLI and server mode
    +-- internal/
    |   +-- models/              shared types (Document, Chunk)
    |   +-- tokenizer/           Tokenizer interface, WhitespaceTokenizer, BPETokenizer
    |   +-- detector/            document type detection
    |   +-- chunker/             structure-aware splitting per document type
    |   +-- pdf/                 PDF text extraction via pdftotext
    |   +-- cli/                 CLI logic
    |   +-- server/              HTTP server
    +-- testdata/                real East African document samples
    +-- docs/architecture.md     technical architecture

---

## Why Go

Single binary. No runtime. No dependency hell. 19 MB Docker image including PDF extraction. Deployable on constrained infrastructure anywhere in East Africa.

---

## Roadmap

- [x] Document type detection (CBK, SACCO, Court, Land Title, Legislation)
- [x] Structure-aware chunking per document type
- [x] PDF input support via pdftotext
- [x] CLI with human-readable, JSON stdout, and JSON file output modes
- [x] HTTP API — POST /chunk with multipart upload
- [x] Embedding-ready output — token counts and overlap control
- [x] Docker deployment
- [ ] Request logging and rate limiting
- [ ] Additional document types (NTSA, KRA, county government notices)
- [ ] Web demo UI
- [ ] WASM build for browser-side chunking

---

*Built in Nairobi. Solving African problems with African context.*
