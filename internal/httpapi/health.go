package httpapi

import "net/http"

func Health(ready func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !ready() {
			Failure(w, r, 503, "not_ready", "server is not ready", nil)
			return
		}
		JSON(w, 200, map[string]string{"status": "ok"})
	}
}
