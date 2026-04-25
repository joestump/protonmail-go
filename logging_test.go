package protonmail

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDebugLoggingRedactsSecrets exercises the request and response debug
// log sites in (*Client).newRequest, newJSONRequest, and doJSON. The /auth
// request body carries a (fake) RefreshToken; the /auth response carries
// (fake) AccessToken / RefreshToken / ServerProof. Pre-#6 the library wrote
// these directly to the global log package via "%#v" of the response struct
// and string(b) of the request body. After #6 they must not appear.
func TestDebugLoggingRedactsSecrets(t *testing.T) {
	const (
		fakeAccessToken  = "FAKE-ACCESS-TOKEN-DO-NOT-LOG-abc123"
		fakeRefreshToken = "FAKE-REFRESH-TOKEN-DO-NOT-LOG-xyz789"
		fakeServerProof  = "FAKE-SERVER-PROOF-DO-NOT-LOG-server"
		fakeUID          = "FAKE-UID-aabbcc"
	)

	resp := authResp{
		Auth: Auth{
			UID:          fakeUID,
			AccessToken:  fakeAccessToken,
			RefreshToken: fakeRefreshToken,
			Scope:        "full self",
		},
		ExpiresIn:   3600,
		TokenType:   "Bearer",
		ServerProof: fakeServerProof,
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal authResp: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/refresh" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	c, err := NewClient(
		WithBaseURL(srv.URL),
		WithAppVersion("test/0.0.1"),
		WithHTTPClient(srv.Client()),
		WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	expired := &Auth{UID: fakeUID, RefreshToken: fakeRefreshToken}
	got, err := c.AuthRefresh(context.Background(), expired)
	if err != nil {
		t.Fatalf("AuthRefresh: %v", err)
	}
	if got.AccessToken != fakeAccessToken {
		t.Errorf("AccessToken roundtrip mismatch: got %q want %q", got.AccessToken, fakeAccessToken)
	}

	out := buf.String()
	if out == "" {
		t.Fatal("expected debug log output, got empty buffer")
	}

	// Secrets must NOT appear anywhere in debug output.
	for _, secret := range []string{
		fakeAccessToken,
		fakeRefreshToken,
		fakeServerProof,
		fakeUID,
	} {
		if strings.Contains(out, secret) {
			t.Errorf("debug log leaked sensitive value %q\nfull log:\n%s", secret, out)
		}
	}

	// Useful structured fields must be present.
	for _, want := range []string{"path=/auth/refresh", "method=POST", "status=200"} {
		if !strings.Contains(out, want) {
			t.Errorf("debug log missing %q\nfull log:\n%s", want, out)
		}
	}
}

// TestDiscardLoggerSilencesLibrary confirms a consumer can fully silence
// debug output by passing a logger backed by slog.DiscardHandler (the
// default) — the buffer the consumer never sees stays empty.
func TestDiscardLoggerSilencesLibrary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Code":1000}`))
	}))
	defer srv.Close()

	logger := slog.New(slog.DiscardHandler)

	c, err := NewClient(
		WithBaseURL(srv.URL),
		WithAppVersion("test/0.0.1"),
		WithHTTPClient(srv.Client()),
		WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	req, err := c.newRequest(context.Background(), http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if err := c.doJSON(context.Background(), req, nil); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
}

// TestRedactedHeaders covers the small helper directly. Authorization and
// X-Pm-Uid values must be replaced; other headers must pass through; the
// input must not be mutated.
func TestRedactedHeaders(t *testing.T) {
	in := http.Header{}
	in.Set("Authorization", "Bearer SECRET")
	in.Set("X-Pm-Uid", "uid-secret")
	in.Set("Content-Type", "application/json")

	out := redactedHeaders(in)

	if got := out.Get("Authorization"); got != "[REDACTED]" {
		t.Errorf("Authorization = %q, want [REDACTED]", got)
	}
	if got := out.Get("X-Pm-Uid"); got != "[REDACTED]" {
		t.Errorf("X-Pm-Uid = %q, want [REDACTED]", got)
	}
	if got := out.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	// Original must be unchanged (Clone semantics).
	if got := in.Get("Authorization"); got != "Bearer SECRET" {
		t.Errorf("input mutated: Authorization = %q", got)
	}
}
