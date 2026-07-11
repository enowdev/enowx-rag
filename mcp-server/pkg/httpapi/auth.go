package httpapi

import (
	"crypto/subtle"
	"net/http"
	"os"
)

// AdminTokenMiddleware returns an HTTP middleware that protects /api/*
// endpoints with a shared admin token. When RAG_ADMIN_TOKEN is set, every
// request to a protected route must include an Authorization header whose
// value matches "Bearer <token>". When the env var is unset, the middleware
// is a no-op (all requests pass through without auth).
//
// The token comparison uses subtle.ConstantTimeCompare to prevent timing
// attacks.
func AdminTokenMiddleware(next http.Handler) http.Handler {
	token := os.Getenv("RAG_ADMIN_TOKEN")
	if token == "" {
		// No token configured: no auth required.
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := extractBearerToken(r)
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts the token from an Authorization header
// formatted as "Bearer <token>". Returns an empty string if the header
// is missing or malformed.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}
