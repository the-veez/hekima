# Contributing to Hekima

Thank you for your interest in contributing. Hekima is open source infrastructure for East African AI systems — every contribution that deepens its understanding of East African documents makes it more valuable for the entire community.

---

## What we need most

### New document types
Hekima currently supports CBK circulars, SACCO policies, court judgments, land titles, and Kenyan legislation. The most valuable contributions are new document types — but only if they are derived from real documents.

If you work with any of the following and have real document samples, open an issue:
- NTSA forms and vehicle registration documents
- KRA tax notices and assessment letters
- County government notices and circulars
- NEMA environmental assessment reports
- SASRA regulatory circulars
- University transcripts and academic documents
- Hospital discharge summaries and medical records

### Improved detection signatures
If Hekima misidentifies a document type you are working with, open an issue with a sample (redacted if sensitive) and the expected type. Detection signatures are in `internal/detector/detector.go`.

### Bug reports
If a document you expected Hekima to handle produces wrong chunk boundaries, open an issue with the document type, a description of the wrong boundary, and what the correct boundary should be.

---

## How to contribute code

### 1. Set up your environment

Prerequisites:
- Go 1.26 or higher
- poppler-utils (`sudo apt install poppler-utils` on Ubuntu/Debian)

```bash
git clone https://github.com/the-veez/hekima.git
cd hekima
go mod download
go test ./...
```

All tests must pass before you start.

### 2. Adding a new document type

Follow this exact sequence — it is the same sequence used for every existing document type:

**Step 1 — Add the constant to `internal/models/models.go`:**
```go
TypeNTSAForm DocumentType = "ntsa_form"
```

**Step 2 — Add detection signatures to `internal/detector/detector.go`:**
Signatures are phrases that appear in the document and nowhere else. Minimum score of 2 required for detection. Choose phrases that are unambiguous — they must not appear in other supported document types.

**Step 3 — Add a splitting strategy to `internal/chunker/chunker.go`:**
Wire it into the `SplitWithOptions` switch statement. The strategy must be derived from the real structural grammar of the document type — not invented. Read at least five real documents before writing the splitter.

**Step 4 — Add a test fixture to `testdata/`:**
A realistic sample document (real or synthetic, redacted if necessary) that covers every structural edge case your splitter handles.

**Step 5 — Add tests:**
- `internal/detector/detector_test.go` — verify correct type detection, and verify no confusion with existing types
- `internal/chunker/chunker_test.go` — verify correct chunk boundaries, metadata completeness, sequential IDs

**Step 6 — Update `docs/architecture.md`:**
Add the new type to the chunking strategies table.

### 3. Code standards

- `gofmt -l .` must be silent before every commit
- `go build ./...` must be clean
- `go test -race -count=1 ./...` must be green
- No stubs, no placeholders, no `// TODO` in submitted code
- Every exported function must have a doc comment
- Errors must be typed where callers need to distinguish them — no string matching

### 4. Commit messages

Use conventional commits:
feat(detector): add TypeNTSAForm detection signatures
feat(chunker): add splitByNTSAFields for NTSA form chunking
test(detector): add TypeNTSAForm detection tests
test(chunker): add TypeNTSAForm chunking tests

One logical change per commit. Do not bundle unrelated changes.

### 5. Pull request checklist

Before opening a PR, verify:
- [ ] `gofmt -l .` is silent
- [ ] `go test -race -count=1 ./...` is green
- [ ] New document type has test fixtures in `testdata/`
- [ ] Detection tests verify no confusion with existing types
- [ ] Chunking tests verify boundaries, metadata, and sequential IDs
- [ ] `docs/architecture.md` is updated

---

## Ground rules

**Real documents only.** Document grammars must be derived from real East African documents. Do not invent structural rules — read the actual documents.

**No silent fallbacks.** Hekima returns `ErrUnknownDocType` for unrecognised documents rather than falling back to generic chunking. New document types must follow the same discipline — if the document does not match, say so explicitly.

**Security is not optional.** File size limits, typed errors, safe file permissions, and input validation are not features — they are baseline requirements. Do not submit code that weakens these properties.

---

## Opening an issue

For bug reports, include:
- Document type (or "unknown" if detection is failing)
- Description of the wrong behaviour
- Expected behaviour
- A sample document or synthetic reproduction (redact sensitive content)

For feature requests, include:
- The document type and its structural grammar
- At least one real document sample
- Why generic chunking fails on this document type

---

*Built in Nairobi. Solving African problems with African context.*
