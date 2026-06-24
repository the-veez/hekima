package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/the-veez/hekima/internal/models"
	"github.com/the-veez/hekima/internal/server"
)

// newChunkRequest builds a multipart POST /chunk request from a file on disk.
func newChunkRequest(t *testing.T, filePath, overlapWords string) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("newChunkRequest: cannot open %s: %v", filePath, err)
	}
	defer f.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		t.Fatalf("newChunkRequest: cannot create form file: %v", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		t.Fatalf("newChunkRequest: cannot copy file content: %v", err)
	}

	if overlapWords != "" {
		if err := writer.WriteField("overlap_words", overlapWords); err != nil {
			t.Fatalf("newChunkRequest: cannot write overlap_words field: %v", err)
		}
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/chunk", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// testdata returns an absolute path to a file in the testdata directory.
func testdata(t *testing.T, name string) string {
	t.Helper()
	// server_test.go lives at internal/server/ — testdata is two levels up.
	path := filepath.Join("..", "..", "testdata", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("testdata: cannot find %s: %v", path, err)
	}
	return path
}

// newServer returns an httptest.Server backed by Hekima's real handler mux.
func newServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chunk", server.HandleChunk)
	mux.HandleFunc("/health", server.HandleHealth)
	return httptest.NewServer(mux)
}

// TestHealth_OK verifies GET /health returns 200 and {"status":"ok"}.
func TestHealth_OK(t *testing.T) {
	srv := newServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health: status = %d, want 200", resp.StatusCode)
	}

	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("GET /health: cannot decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Errorf("GET /health: status = %q, want \"ok\"", payload["status"])
	}
}

// TestHealth_WrongMethod verifies POST /health returns 405.
func TestHealth_WrongMethod(t *testing.T) {
	srv := newServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/health", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /health: status = %d, want 405", resp.StatusCode)
	}
}

// TestChunk_CBKCircular verifies a known CBK circular produces chunks
// with correct structure and populated token counts.
func TestChunk_CBKCircular(t *testing.T) {
	srv := newServer()
	defer srv.Close()

	req := newChunkRequest(t, testdata(t, "cbk_circular.txt"), "")
	resp, err := http.DefaultClient.Do(&http.Request{
		Method: req.Method,
		URL:    mustParseURL(srv.URL + "/chunk"),
		Header: req.Header,
		Body:   req.Body,
	})
	if err != nil {
		t.Fatalf("POST /chunk: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /chunk: status = %d, body = %s", resp.StatusCode, body)
	}

	var chunks []models.Chunk
	if err := json.NewDecoder(resp.Body).Decode(&chunks); err != nil {
		t.Fatalf("POST /chunk: cannot decode response: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("POST /chunk: got 0 chunks")
	}
	for _, c := range chunks {
		if c.TokenCount == 0 {
			t.Errorf("chunk %d (%q): TokenCount is 0", c.ID, c.Section)
		}
		if c.Text == "" {
			t.Errorf("chunk %d: Text is empty", c.ID)
		}
	}
}

// TestChunk_OverlapWords verifies that overlap_words is applied:
// chunk[1].Text must start with words from the tail of chunk[0].Text.
func TestChunk_OverlapWords(t *testing.T) {
	srv := newServer()
	defer srv.Close()

	// baseline — no overlap
	reqBase := newChunkRequest(t, testdata(t, "cbk_circular.txt"), "")
	respBase, _ := http.DefaultClient.Do(&http.Request{
		Method: reqBase.Method,
		URL:    mustParseURL(srv.URL + "/chunk"),
		Header: reqBase.Header,
		Body:   reqBase.Body,
	})
	defer respBase.Body.Close()
	var base []models.Chunk
	json.NewDecoder(respBase.Body).Decode(&base)

	// with overlap
	reqOver := newChunkRequest(t, testdata(t, "cbk_circular.txt"), "10")
	respOver, err := http.DefaultClient.Do(&http.Request{
		Method: reqOver.Method,
		URL:    mustParseURL(srv.URL + "/chunk"),
		Header: reqOver.Header,
		Body:   reqOver.Body,
	})
	if err != nil {
		t.Fatalf("POST /chunk overlap: %v", err)
	}
	defer respOver.Body.Close()
	var over []models.Chunk
	json.NewDecoder(respOver.Body).Decode(&over)

	if len(over) < 2 {
		t.Fatal("need at least 2 chunks to test overlap")
	}
	// chunk[1] with overlap must be longer than chunk[1] without
	if len(over[1].Text) <= len(base[1].Text) {
		t.Errorf("overlap chunk[1] is not longer than baseline: overlap=%d baseline=%d",
			len(over[1].Text), len(base[1].Text))
	}
}

// TestChunk_UnknownDocType verifies that a file Hekima cannot identify
// returns 422, not 500.
func TestChunk_UnknownDocType(t *testing.T) {
	// Write a temp file with no recognisable signatures.
	tmp, err := os.CreateTemp("", "hekima-unknown-*.txt")
	if err != nil {
		t.Fatalf("cannot create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString("this document contains no recognisable East African structural markers")
	tmp.Close()

	srv := newServer()
	defer srv.Close()

	req := newChunkRequest(t, tmp.Name(), "")
	resp, err := http.DefaultClient.Do(&http.Request{
		Method: req.Method,
		URL:    mustParseURL(srv.URL + "/chunk"),
		Header: req.Header,
		Body:   req.Body,
	})
	if err != nil {
		t.Fatalf("POST /chunk unknown: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("POST /chunk unknown: status = %d, want 422, body = %s", resp.StatusCode, body)
	}
}

// TestChunk_MissingFileField verifies that a request with no file field
// returns 400.
func TestChunk_MissingFileField(t *testing.T) {
	srv := newServer()
	defer srv.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("overlap_words", "5")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/chunk", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(&http.Request{
		Method: req.Method,
		URL:    mustParseURL(srv.URL + "/chunk"),
		Header: req.Header,
		Body:   req.Body,
	})
	if err != nil {
		t.Fatalf("POST /chunk no file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /chunk no file: status = %d, want 400", resp.StatusCode)
	}
}

// TestChunk_InvalidOverlapWords verifies that a non-integer overlap_words
// returns 400.
func TestChunk_InvalidOverlapWords(t *testing.T) {
	srv := newServer()
	defer srv.Close()

	req := newChunkRequest(t, testdata(t, "cbk_circular.txt"), "notanumber")
	resp, err := http.DefaultClient.Do(&http.Request{
		Method: req.Method,
		URL:    mustParseURL(srv.URL + "/chunk"),
		Header: req.Header,
		Body:   req.Body,
	})
	if err != nil {
		t.Fatalf("POST /chunk bad overlap: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /chunk bad overlap: status = %d, want 400", resp.StatusCode)
	}
}

// TestChunk_WrongMethod verifies GET /chunk returns 405.
func TestChunk_WrongMethod(t *testing.T) {
	srv := newServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/chunk")
	if err != nil {
		t.Fatalf("GET /chunk: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /chunk: status = %d, want 405", resp.StatusCode)
	}
}

// mustParseURL parses a URL and panics on error — test helper only.
func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
