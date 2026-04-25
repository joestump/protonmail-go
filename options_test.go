package protonmail

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// TestNewClientDefaults verifies that NewClient supplies sensible defaults
// when only the required option is provided.
func TestNewClientDefaults(t *testing.T) {
	c, err := NewClient(WithAppVersion("test/1.0"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.httpClient != http.DefaultClient {
		t.Errorf("httpClient = %v, want http.DefaultClient", c.httpClient)
	}
	if !strings.HasPrefix(c.userAgent, "protonmail-go/") {
		t.Errorf("userAgent = %q, want prefix %q", c.userAgent, "protonmail-go/")
	}
	if strings.Contains(c.userAgent, "Mozilla") || strings.Contains(c.userAgent, "Firefox") {
		t.Errorf("userAgent should not impersonate a browser: %q", c.userAgent)
	}
	if c.logger == nil {
		t.Errorf("logger should default to a non-nil discard logger")
	}
}

// TestNewClientRequiresAppVersion confirms WithAppVersion is mandatory.
func TestNewClientRequiresAppVersion(t *testing.T) {
	if _, err := NewClient(); err == nil {
		t.Fatal("NewClient() with no options: want error, got nil")
	}
	if _, err := NewClient(WithAppVersion("")); err == nil {
		t.Fatal("NewClient(WithAppVersion(\"\")): want error, got nil")
	}
}

// TestWithBaseURLRejectsHTTP confirms http:// is rejected for non-loopback
// hosts but allowed for the exact loopback identifiers 127.0.0.1, ::1, and
// localhost (so tests using httptest.Server work). Broader 127.0.0.0/8
// addresses are deliberately rejected.
func TestWithBaseURLRejectsHTTP(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https remote", "https://api.proton.me", false},
		{"http remote", "http://api.proton.me", true},
		{"http localhost", "http://localhost:8080", false},
		{"http LOCALHOST mixed case", "http://LocalHost:8080", false},
		{"http 127.0.0.1", "http://127.0.0.1:8080", false},
		{"http ipv6 loopback", "http://[::1]:8080", false},
		{"http 127.0.0.2 rejected", "http://127.0.0.2:8080", true},
		{"http 127.1.2.3 rejected", "http://127.1.2.3:8080", true},
		{"ftp scheme", "ftp://api.proton.me", true},
		{"trailing slash trimmed", "https://api.proton.me/", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClient(WithAppVersion("t/1"), WithBaseURL(tc.url))
			if tc.wantErr && err == nil {
				t.Fatalf("WithBaseURL(%q): want error, got nil", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("WithBaseURL(%q): unexpected error: %v", tc.url, err)
			}
		})
	}
}

// TestWithBaseURLTrimsTrailingSlash ensures the trailing slash is normalised
// so callers concatenating paths don't produce "//".
func TestWithBaseURLTrimsTrailingSlash(t *testing.T) {
	c, err := NewClient(WithAppVersion("t/1"), WithBaseURL("https://api.proton.me/"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.baseURL != "https://api.proton.me" {
		t.Errorf("baseURL = %q, want trimmed", c.baseURL)
	}
}

// TestOptionNilGuards confirms options reject nil arguments cleanly.
func TestOptionNilGuards(t *testing.T) {
	if _, err := NewClient(WithAppVersion("t/1"), WithHTTPClient(nil)); err == nil {
		t.Error("WithHTTPClient(nil): want error")
	}
	if _, err := NewClient(WithAppVersion("t/1"), WithReAuth(nil)); err == nil {
		t.Error("WithReAuth(nil): want error")
	}
	if _, err := NewClient(WithAppVersion("t/1"), WithLogger(nil)); err == nil {
		t.Error("WithLogger(nil): want error")
	}
}

// TestWithUserAgentOverride verifies callers can override the default UA.
func TestWithUserAgentOverride(t *testing.T) {
	c, err := NewClient(WithAppVersion("t/1"), WithUserAgent("myapp/2.3"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.userAgent != "myapp/2.3" {
		t.Errorf("userAgent = %q, want %q", c.userAgent, "myapp/2.3")
	}
}

// TestWithReAuthInstalled sanity-checks that the callback is stored and
// invokable. The actual 401-retry path is covered by auth_test.go.
func TestWithReAuthInstalled(t *testing.T) {
	called := false
	fn := func(ctx context.Context) error { called = true; return nil }
	c, err := NewClient(WithAppVersion("t/1"), WithReAuth(fn))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.reauth == nil {
		t.Fatal("reauth callback not installed")
	}
	_ = c.reauth(context.Background())
	if !called {
		t.Error("reauth callback was not invoked through stored field")
	}
}

// TestWithLoggerCustom confirms the logger field is stored.
func TestWithLoggerCustom(t *testing.T) {
	l := slog.New(slog.DiscardHandler)
	c, err := NewClient(WithAppVersion("t/1"), WithLogger(l))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.logger != l {
		t.Error("custom logger not stored")
	}
}
