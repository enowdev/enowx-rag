package httpapi

import (
	"crypto/subtle"
	"net"
	"net/http"

	"github.com/enowdev/enowx-rag/pkg/config"
)

// AdminTokenMiddleware returns an HTTP middleware that protects /api/* and /mcp
// with a shared admin token. The effective token is RAG_ADMIN_TOKEN if set,
// otherwise the value saved in config.yaml (config.EffectiveAdminToken). It is
// read per-request so a token generated at runtime takes effect immediately.
// When there is no token, the middleware is a no-op (no auth).
//
// The token comparison uses subtle.ConstantTimeCompare to prevent timing attacks.
func AdminTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := config.EffectiveAdminToken()
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
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
		token := config.EffectiveAdminToken()
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
