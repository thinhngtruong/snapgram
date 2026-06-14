package httpapi

import (
	"net/http"
	"time"
)

func (api *API) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		api.deps.Logger.Printf("request method=%s path=%s duration=%s", r.Method, r.URL.Path, time.Since(start))
	})
}

func (api *API) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				api.deps.Logger.Printf("panic recovered value=%v", value)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
