package middleware

import (
	"compress/gzip"
	"net/http"

	"github.com/opentrace/opentrace/pkg/schema"
)

// Decompress transparently inflates request bodies sent with
// Content-Encoding: gzip (the OpenTrace SDK compresses batches > 1KB).
//
// Ordering matters: this middleware must run BEFORE any handler applies
// http.MaxBytesReader, so the payload limit is enforced against the
// DECOMPRESSED byte stream. That ordering is the gzip-bomb defence — a
// tiny compressed body that inflates past the limit is cut off at the
// limit, not buffered to completion.
func Decompress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "gzip" {
			next.ServeHTTP(w, r)
			return
		}
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			schema.WriteProblem(w, schema.Problem{
				Type:   "https://opentrace.io/errors/invalid-encoding",
				Title:  "Bad Request",
				Status: http.StatusBadRequest,
				Detail: "Content-Encoding is gzip but the body is not valid gzip data.",
			})
			return
		}
		defer gz.Close()

		r.Body = gz
		// The body is no longer the on-wire representation: downstream
		// consumers must not re-interpret it as compressed, and the
		// original Content-Length describes the compressed stream.
		r.Header.Del("Content-Encoding")
		r.Header.Del("Content-Length")
		r.ContentLength = -1

		next.ServeHTTP(w, r)
	})
}
