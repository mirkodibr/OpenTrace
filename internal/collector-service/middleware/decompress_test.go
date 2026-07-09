package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// echoHandler mimics the ingest handler's body handling: MaxBytesReader
// then a full read.
func echoHandler(limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			if err.Error() == "http: request body too large" {
				http.Error(w, "too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

func TestDecompressGzipBody(t *testing.T) {
	payload := []byte(`{"events":[{"body":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewReader(gzipBytes(t, payload)))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	Decompress(echoHandler(1<<20)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != string(payload) {
		t.Fatalf("body = %q, want %q", got, payload)
	}
}

func TestDecompressPassthroughUncompressed(t *testing.T) {
	payload := []byte(`{"events":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	Decompress(echoHandler(1<<20)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != string(payload) {
		t.Fatalf("body = %q, want %q", got, payload)
	}
}

func TestDecompressRejectsCorruptGzip(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", strings.NewReader("definitely not gzip"))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	Decompress(echoHandler(1<<20)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
}

// TestDecompressGzipBombCutOffAtLimit is the security-critical ordering
// test: a small compressed body inflating far past the limit must be
// rejected by MaxBytesReader on the DECOMPRESSED stream, never buffered
// to completion.
func TestDecompressGzipBombCutOffAtLimit(t *testing.T) {
	// 10 MB of zeros compresses to ~10 KB.
	bomb := gzipBytes(t, make([]byte, 10<<20))
	if len(bomb) > 100<<10 {
		t.Fatalf("test setup: bomb should compress well, got %d bytes", len(bomb))
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewReader(bomb))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	Decompress(echoHandler(1<<20 /* 1 MB decompressed limit */)).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (bomb must hit the decompressed-bytes limit)", rec.Code)
	}
}

func TestDecompressStripsEncodingHeaders(t *testing.T) {
	payload := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(gzipBytes(t, payload)))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	var seen *http.Request
	Decompress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if seen.Header.Get("Content-Encoding") != "" {
		t.Error("Content-Encoding not stripped after decompression")
	}
	if seen.ContentLength != -1 {
		t.Errorf("ContentLength = %d, want -1 (compressed length is meaningless downstream)", seen.ContentLength)
	}
}
