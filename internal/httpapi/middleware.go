package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"banner-fingerprint/internal/model"
)

type requestIDKey struct{}

var sequence atomic.Uint64

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func Failure(w http.ResponseWriter, r *http.Request, status int, code, message string, index *int) {
	JSON(w, status, model.APIError{Error: model.ErrorDetail{
		Code: code, Message: message, Index: index, RequestID: RequestID(r.Context()),
	}})
}

// Responses are buffered so a panic cannot leave a partial successful response.
type responseBuffer struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *responseBuffer) Header() http.Header { return w.header }
func (w *responseBuffer) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}
func (w *responseBuffer) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.body.Write(p)
}

func Middleware(logger *slog.Logger, maxBody int64, maxConcurrent int, next http.Handler) http.Handler {
	slots := make(chan struct{}, maxConcurrent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var raw [16]byte
		_, _ = rand.Read(raw[:])
		id := hex.EncodeToString(raw[:])
		// Include a monotonic suffix even if the OS random source is unavailable.
		id += "-" + strconv.FormatUint(sequence.Add(1), 16)
		ctx, cancel := context.WithTimeout(context.WithValue(r.Context(), requestIDKey{}, id), 25*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		b := &responseBuffer{header: make(http.Header)}
		b.Header().Set("X-Request-ID", id)
		b.Header().Set("X-Content-Type-Options", "nosniff")
		defer func() {
			if recover() != nil {
				b = &responseBuffer{header: make(http.Header)}
				b.Header().Set("X-Request-ID", id)
				b.Header().Set("X-Content-Type-Options", "nosniff")
				Failure(b, r, http.StatusInternalServerError, "internal_error", "internal server error", nil)
			}
			if b.code == 0 {
				b.code = http.StatusOK
			}
			for k, values := range b.header {
				w.Header()[k] = values
			}
			w.WriteHeader(b.code)
			_, _ = io.Copy(w, &b.body)
			logger.Info("request", "request_id", id, "method", r.Method, "path", r.URL.Path,
				"status", b.code, "duration_ms", time.Since(start).Milliseconds())
		}()
		r.Body = http.MaxBytesReader(b, r.Body, maxBody)
		if r.Method == http.MethodPost && r.URL.Path == "/fingerprint" {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				Failure(b, r, http.StatusServiceUnavailable, "busy", "too many concurrent requests", nil)
				return
			}
		}
		next.ServeHTTP(b, r)
	})
}
