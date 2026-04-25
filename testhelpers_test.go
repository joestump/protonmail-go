package protonmail

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// readFixture returns the bytes of testdata/<rel>. Fails the test on error.
func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	path := filepath.Join("testdata", rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFixture(%q): %v", rel, err)
	}
	return b
}

// newTestServer wires an httptest.Server and a *Client whose RootURL points
// at it. The handler is invoked for every request — tests can assert on
// method/path/body inside the handler.
//
// The returned cleanup function should be deferred. It closes the server.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := &Client{
		RootURL:    srv.URL,
		AppVersion: "test/0.0.1",
	}
	return srv, c, srv.Close
}
