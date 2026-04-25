//go:build integration

// Package protonmail integration tests.
//
// These exercise the live Proton API and are gated behind the `integration`
// build tag so they never run in the default `go test ./...` suite.
//
// To run:
//
//	export PROTONMAIL_TEST_USERNAME=...   # full Proton address
//	export PROTONMAIL_TEST_PASSWORD=...   # account password
//	go test -tags=integration ./...
//
// Tests skip silently if either env var is unset, so the suite still
// compiles and "passes" in CI environments without credentials. CONTRIBUTING
// will document this convention once #5 lands (#7 acceptance defers to it).
package protonmail

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

const liveRootURL = "https://mail.proton.me/api"

// TestIntegrationListMessages authenticates with credentials from the
// environment and lists up to 5 messages. Acts as a smoke test that the
// auth + list flow works end-to-end against the real API.
//
// This is a placeholder to prove the build tag is wired correctly. It is
// not yet runnable in CI.
func TestIntegrationListMessages(t *testing.T) {
	username := os.Getenv("PROTONMAIL_TEST_USERNAME")
	password := os.Getenv("PROTONMAIL_TEST_PASSWORD")
	if username == "" || password == "" {
		t.Skip("PROTONMAIL_TEST_USERNAME / PROTONMAIL_TEST_PASSWORD not set; skipping integration test")
	}

	c := &Client{
		RootURL:    liveRootURL,
		AppVersion: "Other",
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}

	ctx := context.Background()
	auth, err := c.Auth(ctx, username, password, nil)
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	t.Logf("authenticated as user %q", auth.UserID)

	total, msgs, err := c.ListMessages(ctx, &MessageFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	t.Logf("inbox total=%d, fetched=%d", total, len(msgs))

	if err := c.Logout(ctx); err != nil {
		t.Errorf("Logout: %v", err)
	}
}
