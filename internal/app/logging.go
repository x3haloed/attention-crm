package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type requestIDKey struct{}

var requestIDFallbackCounter uint64

func requestIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(requestIDKey{})
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	return s, s != ""
}

func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id, ok := requestIDFromContext(r.Context()); ok {
		return id
	}
	return ""
}

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = newRequestID()
		} else if len(id) > 128 {
			id = newRequestID()
		}

		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err == nil {
		return base64.RawURLEncoding.EncodeToString(buf)
	}
	seq := atomic.AddUint64(&requestIDFallbackCounter, 1)
	// Non-crypto fallback. Still unique enough for correlating logs.
	return fmt.Sprintf("fallback-%d-%d", time.Now().UTC().UnixNano(), seq)
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	bytes       int
}

func (w *statusWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.status = statusCode
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if rec := recover(); rec != nil {
				id := requestID(r)
				log.Printf("panic rid=%s method=%s path=%s panic=%v", id, r.Method, r.URL.Path, rec)
				if !sw.wroteHeader {
					http.Error(sw, "internal error (request_id="+id+")", http.StatusInternalServerError)
				}
			}

			id := requestID(r)
			tenantSlug, _, ok := parseTenantPath(r.URL.Path)
			if !ok {
				tenantSlug = ""
			}
			dur := time.Since(start)
			log.Printf(
				"rid=%s ip=%s tenant=%s method=%s path=%s status=%d bytes=%d dur_ms=%d",
				id,
				s.clientIP(r),
				tenantSlug,
				r.Method,
				r.URL.Path,
				sw.status,
				sw.bytes,
				dur.Milliseconds(),
			)
		}()

		next.ServeHTTP(sw, r)
	})
}
