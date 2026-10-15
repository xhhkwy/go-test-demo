package httpapi

import (
	"errors"
	"mime"
	"net/http"

	"banner-fingerprint/internal/config"
	"banner-fingerprint/internal/fingerprint"
	"banner-fingerprint/internal/input"
	"banner-fingerprint/internal/model"
)

func Router(engine *fingerprint.Engine, c config.Server, ready func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", Health(ready))
	mux.HandleFunc("POST /fingerprint", func(w http.ResponseWriter, r *http.Request) {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			Failure(w, r, 415, "unsupported_media_type", "Content-Type must be application/json", nil)
			return
		}
		batch, err := input.Parse(r.Body, c.MaxBodyBytes, c.MaxBatch, c.MaxBannerBytes)
		if err != nil {
			var detail *input.Error
			if errors.As(err, &detail) {
				status, code := 400, "invalid_input"
				if detail.TooLarge {
					status, code = 413, "limit_exceeded"
				}
				Failure(w, r, status, code, detail.Message, detail.Index)
				return
			}
			Failure(w, r, 400, "invalid_input", "cannot parse input", nil)
			return
		}
		results := make([]model.Result, 0, len(batch.Scans))
		for i, s := range batch.Scans {
			if err := r.Context().Err(); err != nil {
				Failure(w, r, 504, "request_cancelled", "request cancelled or deadline exceeded", nil)
				return
			}
			results = append(results, engine.Recognize(s, batch.Banners[i]))
		}
		JSON(w, 200, results)
	})
	return mux
}
