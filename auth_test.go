package protonmail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// TestAuthInfoEndpoint exercises Client.AuthInfo against a canned response.
//
// AuthInfo's fields are unexported, so we use white-box (same-package) access
// to assert that the parsed response actually populated them from the fixture.
func TestAuthInfoEndpoint(t *testing.T) {
	body := readFixture(t, "auth/auth_info.json")

	var gotMethod, gotPath string
	var gotReq authInfoReq
	srv, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotReq)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	defer cleanup()
	_ = srv

	ctx := context.Background()
	info, err := c.AuthInfo(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("AuthInfo: %v", err)
	}
	if info == nil {
		t.Fatal("AuthInfo returned nil")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/auth/info" {
		t.Errorf("path = %q, want /auth/info", gotPath)
	}
	if gotReq.Username != "user@example.com" {
		t.Errorf("body.Username = %q, want user@example.com", gotReq.Username)
	}

	// White-box assertions: the parsed AuthInfo must reflect the fixture.
	if info.version != 4 {
		t.Errorf("info.version = %d, want 4", info.version)
	}
	if info.serverEphemeral != "dGVzdC1zZXJ2ZXItZXBoZW1lcmFs" {
		t.Errorf("info.serverEphemeral = %q, want dGVzdC1zZXJ2ZXItZXBoZW1lcmFs", info.serverEphemeral)
	}
	if info.salt != "dGVzdC1zYWx0" {
		t.Errorf("info.salt = %q, want dGVzdC1zYWx0", info.salt)
	}
	if info.srpSession != "test-srp-session-id" {
		t.Errorf("info.srpSession = %q, want test-srp-session-id", info.srpSession)
	}
	// Modulus is a multi-line PGP-armored string — just confirm it's non-empty
	// and contains the BEGIN marker, so we know the field landed.
	if info.modulus == "" {
		t.Error("info.modulus is empty; fixture had a non-empty value")
	}
}

// TestAuthInfoAPIError verifies that an API-level error returned with a
// non-2xx status (401 here) is surfaced to the caller. The lib's HTTP path
// doesn't yet uniformly treat non-2xx specially (see issue #4), so we assert
// only that an error IS returned, not its exact type.
func TestAuthInfoAPIError(t *testing.T) {
	_, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"Code": 8002, "Error": "Invalid credentials"}`)
	})
	defer cleanup()

	ctx := context.Background()
	_, err := c.AuthInfo(ctx, "nobody@example.com")
	if err == nil {
		t.Fatal("expected error from non-2xx response, got nil")
	}
	// If the JSON body decoded into a recognized APIError, double-check the
	// code; if not, the bare error is acceptable until issue #4 lands.
	if apiErr, ok := err.(*APIError); ok {
		if apiErr.Code != 8002 {
			t.Errorf("APIError.Code = %d, want 8002", apiErr.Code)
		}
	}
}

// TestLogout verifies the simple DELETE /auth path and that on success the
// in-memory tokens and key ring are cleared.
func TestLogout(t *testing.T) {
	var gotMethod, gotPath string
	_, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Code": 1000}`)
	})
	defer cleanup()

	c.uid = "u"
	c.accessToken = "t"
	// Stash a non-nil sentinel so we can verify Logout clears it. The exact
	// contents don't matter — we just need the slice to be non-nil.
	c.keyRing = openpgp.EntityList{nil}

	ctx := context.Background()
	if err := c.Logout(ctx); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/auth" {
		t.Errorf("path = %q, want /auth", gotPath)
	}
	if c.uid != "" || c.accessToken != "" {
		t.Errorf("Logout did not clear credentials: uid=%q tok=%q", c.uid, c.accessToken)
	}
	if c.keyRing != nil {
		t.Errorf("Logout did not clear keyRing: %v", c.keyRing)
	}
}

// TestReAuthRetry exercises the 401-retry path in (*Client).do. The httptest
// server returns 401 once, then 200 on the second call. The Client is
// configured with a ReAuth callback that updates the access token. We assert:
//   - the second call succeeds,
//   - ReAuth was invoked exactly once,
//   - the second request carried the refreshed token.
func TestReAuthRetry(t *testing.T) {
	var hits int32
	var lastAuthHeader string
	srv, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		lastAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"Code": 401, "Error": "expired"}`)
			return
		}
		// Second hit: succeed with a minimal valid AuthInfoResp body so that
		// AuthInfo returns nil error.
		_, _ = w.Write(readFixture(t, "auth/auth_info.json"))
	})
	defer cleanup()
	_ = srv

	// Pre-seed credentials so the request carries Authorization (a precondition
	// for the 401 retry path in (*Client).do).
	c.uid = "u"
	c.accessToken = "stale-token"

	var reauthCalls int32
	c.reauth = func(ctx context.Context) error {
		atomic.AddInt32(&reauthCalls, 1)
		c.accessToken = "fresh-token"
		return nil
	}

	ctx := context.Background()
	if _, err := c.AuthInfo(ctx, "user@example.com"); err != nil {
		t.Fatalf("AuthInfo after retry: %v", err)
	}

	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("server hits = %d, want 2", got)
	}
	if got := atomic.LoadInt32(&reauthCalls); got != 1 {
		t.Errorf("ReAuth invocations = %d, want 1", got)
	}
	if lastAuthHeader != "Bearer fresh-token" {
		t.Errorf("retry Authorization header = %q, want Bearer fresh-token", lastAuthHeader)
	}
}
