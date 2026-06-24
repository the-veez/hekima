## Overview

Hekima is a domain-specific document chunking engine for East African AI systems.
It solves the problem that generic RAG chunkers (fixed character counts, sentence
boundaries) destroy document structure in CBK circulars, SACCO policies, court
judgments, and Kenyan legislation. The fix: detect document type first, then apply
the correct structural cutting grammar for that type.

## Dependency Graph

models      → (nothing)

tokenizer   → (nothing)

detector    → models

chunker     → models, tokenizer

pdf         → (stdlib only)

cli         → models, detector, chunker, pdf, tokenizer

main        → cli

No circular dependencies. Each package is independently importable.

## Package Responsibilities

| Package | Responsibility |
|---|---|
| `internal/models` | Single source of truth for all shared types (`Document`, `Chunk`, `DocumentType`) |
| `internal/tokenizer` | `Tokenizer` interface + `WhitespaceTokenizer` (default) + `BPETokenizer` (optional) |
| `internal/detector` | Lexical fingerprint detection — deterministic, stateless, no ML |
| `internal/chunker` | Structure-aware splitting per document type via `Split` and `SplitWithOptions` |
| `internal/pdf` | PDF text extraction via `pdftotext` (poppler-utils) |
| `internal/cli` | All CLI logic — `main.go` calls `cli.Run()` and nothing else |
| `cmd/hekima` | Four-line `main.go` — calls `cli.Run`, prints error, exits |

## Chunking Strategies

| Document Type | Strategy | Boundary Signal |
|---|---|---|
| `sacco_policy` | `splitByHeadings` | ALL-CAPS heading lines |
| `cbk_circular` | `splitByNumberedSections` | Top-level integer sections only — subsections stay inside parent |
| `court_judgment` | `splitByCourtMarkers` | BACKGROUND, FACTS, ANALYSIS, FINDINGS, ORDER |
| `land_title` | `splitByParagraphs` | Blank line boundaries |
| `legislation` | `splitLegislation` | Part headers + Section headers, TOC stripped |
| `unknown` | — | Returns `ErrUnknownDocType` — never silently falls back |

## SplitWithOptions

`SplitWithOptions(doc, opts)` is the primary entry point. `Split(doc)` is a thin
wrapper calling `SplitWithOptions(doc, Options{})` — all existing call sites are
unchanged.

`Options` fields:

- `OverlapWords int` — words to repeat at the start of each chunk from the end of
  the previous chunk. Word boundaries are used (not BPE tokens). 0 disables overlap.
  Negative values are treated as 0.
- `Tokenizer tokenizer.Tokenizer` — used to populate `TokenCount` on every chunk.
  Defaults to `WhitespaceTokenizer` (word count × 1.3, rounded up). Swap in
  `BPETokenizer` for exact counts.

Every emitted `Chunk` carries `TokenCount` (no `omitempty`) so downstream consumers
can rely on its presence without treating zero as "unknown".

## Roadmap

| Priority | Feature | Status |
|---|---|---|
| P0 | Document type detection | ✅ Done |
| P0 | Structure-aware chunking | ✅ Done |
| P0 | CLI with three output modes | ✅ Done |
| P0 | Architecture documentation | ✅ Done |
| P0 | PDF input support | ✅ Done |
| P1 | TypeLegislation — Acts and Statutes, alphanumeric section numbers | ✅ Done |
| P1 | Tests for pdf package | ✅ Done |
| P1 | Tests for cli package | ✅ Done |
| P1 | Embedding-ready output (token counts, overlap control) | ✅ Done |
| P2 | HTTP server mode for pipeline integration | 🔲 Planned |
| P2 | Additional document types (NTSA forms, KRA notices) | 🔲 Planned |
| P3 | WASM build for browser-side chunking | 🔲 Planned |
