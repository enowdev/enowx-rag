package httpapi

import (
	"crypto/subtle"
	"net"
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

// LocalOrAdminMiddleware protects sensitive write endpoints (e.g. the setup
// wizard, which writes ~/.enowx-rag/config.yaml containing API keys). The
// request is allowed when it originates from loopback (the common local-first
// case) OR carries a valid RAG_ADMIN_TOKEN. A remote request without the token
// is rejected, so an exposed instance cannot have its config rewritten or its
// secrets probed by anonymous callers.
func LocalOrAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLoopback(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		token := os.Getenv("RAG_ADMIN_TOKEN")
		provided := extractBearerToken(r)
		if token != "" && provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("WWW-Authenticate", "Bearer")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"setup is restricted to localhost or requires a valid admin token"}`))
	})
}

// isLoopback reports whether the request's remote address is a loopback IP.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
