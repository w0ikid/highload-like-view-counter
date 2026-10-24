package rest

import (
	"context"
	"encoding/json"
	"highload-counters/pkg/traceid"
	"net/http"
	"time"
)

func NewRouter(ready func(context.Context) error, timeout time.Duration) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		id := traceid.New()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := ready(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "unavailable", "message": "dependency unavailable", "requestId": id})
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	return mux
}
