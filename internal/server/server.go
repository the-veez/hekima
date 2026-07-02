// Package server implements Hekima's HTTP API.
//
// It exposes two endpoints:
//
//	POST /chunk   — accepts a multipart/form-data upload, returns JSON chunks
//	GET  /health  — returns {"status":"ok"} for load balancer / uptime checks
//
// The server is intentionally thin: it validates the request, delegates
// to the same detect→chunk pipeline that the CLI uses, and formats the
// response. No business logic lives here.
package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/the-veez/hekima/internal/chunker"
	"github.com/the-veez/hekima/internal/detector"
	"github.com/the-veez/hekima/internal/middleware"
	"github.com/the-veez/hekima/internal/pdf"
)

// maxUploadBytes is the maximum file size accepted by the HTTP server.
// Matches the CLI's plain-text limit; PDF files have their own limit
// enforced inside the pdf package.
const maxUploadBytes = 10 * 1024 * 1024 // 10 MB

//go:embed static/index.html
var indexHTML []byte

// Run starts the HTTP server on the given address (e.g. ":8080").
// It blocks until the server exits.
func Run(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/chunk", HandleChunk)
	mux.HandleFunc("/health", HandleHealth)
	mux.HandleFunc("/", HandleDemo)

	// Apply middleware: rate limiter wraps the mux, logger wraps that.
	// Order matters: logger runs first so it captures the real status
	// code including 429s from the rate limiter.
	// 10 requests per minute per IP (0.1667 rps), burst of 5.
	handler := middleware.Logger(middleware.RateLimiter(10.0/60.0, 5)(mux))

	log.Printf("hekima: listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}

// HandleDemo serves the web demo UI at GET /.
// The HTML is embedded at compile time via go:embed — no static files
// need to exist at runtime, and the Docker image stays a single binary.
func HandleDemo(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(indexHTML)
}

// handleHealth responds to GET /health with a simple liveness payload.
// Used by load balancers, Docker health checks, and uptime monitors.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed — use GET /health")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
}

// handleChunk processes POST /chunk requests.
//
// Expected request: multipart/form-data with:
//   - file         — the document to chunk (required)
//   - overlap_words — number of words to overlap between chunks (optional, default 0)
//
// Response on success: 200 with a JSON array of Chunk objects.
// Response on unknown document type: 422 with a JSON error.
// Response on all other errors: 400 or 500 with a JSON error.
func HandleChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed — use POST /chunk")
		return
	}

	// Cap memory used to buffer the upload. Files larger than 10 MB
	// are written to a temp file by ParseMultipartForm; we reject them
	// below after reading the header.
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	// --- overlap_words ---
	overlapWords := 0
	if raw := r.FormValue("overlap_words"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "overlap_words must be a non-negative integer")
			return
		}
		overlapWords = n
	}

	// --- file ---
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing required form field: file")
		return
	}
	defer file.Close()

	if header.Size > maxUploadBytes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"file %q is %d bytes, exceeding the %d byte limit",
			header.Filename, header.Size, maxUploadBytes,
		))
		return
	}

	// --- extract text ---
	text, err := extractText(file, header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "text extraction failed: "+err.Error())
		return
	}

	// --- detect → chunk ---
	doc := detector.Detect(header.Filename, text)
	chunks, err := chunker.SplitWithOptions(doc, chunker.Options{
		OverlapWords: overlapWords,
	})
	if err != nil {
		if errors.Is(err, chunker.ErrUnknownDocType) {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
				"could not identify document type for %q — "+
					"Hekima supports: cbk_circular, sacco_policy, "+
					"court_judgment, land_title, legislation",
				header.Filename,
			))
			return
		}
		writeError(w, http.StatusInternalServerError, "chunking failed: "+err.Error())
		return
	}

	if len(chunks) == 0 {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"document %q produced zero chunks: file may be empty or contain only whitespace",
			header.Filename,
		))
		return
	}

	writeJSON(w, http.StatusOK, chunks)
}

// extractText reads the uploaded file and returns plain text.
// PDF files are written to a temp file and processed via pdftotext.
// All other files are read directly as UTF-8 text.
func extractText(file io.Reader, filename string) (string, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return extractPDFText(file, filename)
	}

	raw, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	return string(raw), nil
}

// extractPDFText writes the uploaded PDF to a temp file, runs pdftotext,
// and cleans up. The temp file is necessary because pdftotext requires
// a file path, not a stream.
func extractPDFText(file io.Reader, filename string) (string, error) {
	tmp, err := os.CreateTemp("", "hekima-*.pdf")
	if err != nil {
		return "", fmt.Errorf("cannot create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		return "", fmt.Errorf("cannot write temp file: %w", err)
	}
	// Close before passing path to pdftotext — some OS implementations
	// require the file to be closed before another process can read it.
	tmp.Close()

	text, err := pdf.ExtractText(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("PDF extraction failed for %q: %w", filepath.Base(filename), err)
	}
	return text, nil
}

// apiError is the JSON shape returned on all error responses.
type apiError struct {
	Error string `json:"error"`
}

// writeError writes a JSON error response with the given status code.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	out, _ := json.Marshal(apiError{Error: message})
	w.Write(out)
}

// writeJSON writes a JSON success response with the given status code.
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(w, `{"error":"JSON serialization failed: %s"}`, err.Error())
		return
	}
	w.Write(out)
}
