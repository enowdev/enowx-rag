package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAdminToken_Unset_NoAuth verifies that when no admin token is configured
// (neither RAG_ADMIN_TOKEN nor config.yaml), requests pass through unauthenticated.
func TestAdminToken_Unset_NoAuth(t *testing.T) {
	// Isolate from the host: empty HOME means no config token, and clear the env.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("RAG_ADMIN_TOKEN", "")

	called := false
	h := AdminTokenMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !called {
		t.Error("expected handler to be called when RAG_ADMIN_TOKEN is unset")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestAdminToken_Set_NoHeader_Returns401 verifies that when RAG_ADMIN_TOKEN
// is set and no Authorization header is provided, the request is rejected
// with 401.
func TestAdminToken_Set_NoHeader_Returns401(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "secret-token-123")

	called := false
	h := AdminTokenMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called when auth header is missing")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !contains(w.Body.String(), "unauthorized") {
		t.Errorf("expected 'unauthorized' in body, got: %s", w.Body.String())
	}
}

// TestAdminToken_Set_WrongToken_Returns401 verifies that when RAG_ADMIN_TOKEN
// is set and the Authorization header contains a wrong token, the request is
// rejected with 401.
func TestAdminToken_Set_WrongToken_Returns401(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "secret-token-123")

	called := false
	h := AdminTokenMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called when token is wrong")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestAdminToken_Set_CorrectToken_PassesThrough verifies that when
// RAG_ADMIN_TOKEN is set and the Authorization header contains the correct
// token, the request passes through to the handler.
func TestAdminToken_Set_CorrectToken_PassesThrough(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "secret-token-123")

	called := false
	h := AdminTokenMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !called {
		t.Error("expected handler to be called when correct token is provided")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestAdminToken_Set_MalformedHeader_Returns401 verifies that a malformed
// Authorization header (not "Bearer <token>") is rejected with 401.
func TestAdminToken_Set_MalformedHeader_Returns401(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "secret-token-123")

	called := false
	h := AdminTokenMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Basic secret-token-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called when auth header is malformed")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestAdminToken_RouterIntegration verifies the auth middleware works
// end-to-end with the chi router: with token set, /api/projects returns 401
// without auth and 200 with correct auth; SPA routes are never blocked.
func TestAdminToken_RouterIntegration(t *testing.T) {
	p := &mockProvider{projects: []string{}}
	_, router := newTestServer(t, p, nil)
	// Set the token AFTER newTestServer (which clears it for isolation). Auth is
	// read per-request, so this takes effect for the requests below.
	t.Setenv("RAG_ADMIN_TOKEN", "test-admin-token")

	// Without auth header -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}

	// With correct auth header -> 200
	req2 := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req2.Header.Set("Authorization", "Bearer test-admin-token")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 with correct auth, got %d", w2.Code)
	}

	// With wrong auth header -> 401
	req3 := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req3.Header.Set("Authorization", "Bearer wrong")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong auth, got %d", w3.Code)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
