package protonmail

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// trackingBody wraps an io.ReadCloser and records whether Close was called.
type trackingBody struct {
	io.ReadCloser
	closed *atomic.Int32
}

func (b *trackingBody) Close() error {
	b.closed.Add(1)
	return b.ReadCloser.Close()
}

// trackingTransport wraps an http.RoundTripper and replaces every response
// body with a trackingBody so tests can assert that callers (or the client)
// closed the body.
type trackingTransport struct {
	inner    http.RoundTripper
	closed   atomic.Int32
	bodies   atomic.Int32 // total responses returned with a non-nil body
}

func (t *trackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp.Body != nil {
		t.bodies.Add(1)
		resp.Body = &trackingBody{ReadCloser: resp.Body, closed: &t.closed}
	}
	return resp, nil
}

// TestDoReturnsBodyForCallerOn200 verifies do() returns a readable body on
// 200 and that the caller is the one who closes it.
func TestDoReturnsBodyForCallerOn200(t *testing.T) {
	srv, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	})
	defer cleanup()
	_ = srv

	req, err := c.newRequest(context.Background(), http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	resp, err := c.do(context.Background(), req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "hello world" {
		t.Errorf("body = %q, want %q", body, "hello world")
	}
}

// TestDoTransportErrorReturnsNilResponse verifies that do() returns
// (nil, err) on transport errors so callers don't accidentally try to
// dereference a non-nil response.
func TestDoTransportErrorReturnsNilResponse(t *testing.T) {
	c, err := NewClient(
		// Bogus URL; net/http will fail to connect. Loopback exemption permits http.
		WithBaseURL("http://127.0.0.1:1"),
		WithAppVersion("test/0.0.1"),
		WithHTTPClient(&http.Client{Transport: &http.Transport{}}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	req, err := c.newRequest(context.Background(), http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	resp, err := c.do(context.Background(), req)
	if err == nil {
		t.Fatalf("expected transport error, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response on transport error, got %#v", resp)
	}
}

// TestDoJSONNonJSON401Body verifies that doJSON, given a 401 with a
// non-JSON body (e.g. an HTML error page), returns a useful non-nil error
// instead of a JSON decode error, and does not panic.
func TestDoJSONNonJSON401Body(t *testing.T) {
	const html = "<html><body>nope</body></html>"
	srv, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(html))
	})
	defer cleanup()
	_ = srv

	req, err := c.newRequest(context.Background(), http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	var out struct{ X int }
	err = c.doJSON(context.Background(), req, &out)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q does not mention 401", err.Error())
	}
	// Ensure it's not a JSON syntax error from trying to decode HTML.
	if strings.Contains(err.Error(), "invalid character '<'") {
		t.Errorf("error looks like a raw JSON decode failure: %v", err)
	}
}

// TestDoJSONEmpty500 verifies that doJSON on a 500 with an empty body
// surfaces a useful error instead of "EOF".
func TestDoJSONEmpty500(t *testing.T) {
	srv, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer cleanup()
	_ = srv

	req, err := c.newRequest(context.Background(), http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	var out struct{ X int }
	err = c.doJSON(context.Background(), req, &out)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention 500", err.Error())
	}
	if errors.Is(err, io.EOF) {
		t.Errorf("error is bare io.EOF, expected a status-bearing error")
	}
}

// TestDoJSONAPIErrorEnvelopeStillTyped verifies that when a non-2xx body
// happens to parse as a Proton API error envelope, doJSON still returns
// the existing *APIError shape so existing callers keep working.
func TestDoJSONAPIErrorEnvelopeStillTyped(t *testing.T) {
	srv, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"Code":1234,"Error":"bad thing"}`))
	})
	defer cleanup()
	_ = srv

	req, err := c.newRequest(context.Background(), http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	var out struct{ X int }
	err = c.doJSON(context.Background(), req, &out)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != 1234 || apiErr.Message != "bad thing" {
		t.Errorf("APIError = %+v, want Code=1234 Message=\"bad thing\"", apiErr)
	}
}

// TestGetAttachmentNon2xxClosesBody verifies that GetAttachment, when the
// API returns a non-2xx, has closed the response body before returning. We
// detect this with a trackingTransport that wraps every response body.
func TestGetAttachmentNon2xxClosesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html>not found</html>"))
	}))
	defer srv.Close()

	tt := &trackingTransport{inner: http.DefaultTransport}
	c, err := NewClient(
		WithBaseURL(srv.URL),
		WithAppVersion("test/0.0.1"),
		WithHTTPClient(&http.Client{Transport: tt}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	rc, err := c.GetAttachment(context.Background(), "att-id")
	if err == nil {
		t.Fatalf("expected error, got nil and rc=%v", rc)
	}
	if rc != nil {
		t.Errorf("expected nil ReadCloser on error, got %v", rc)
		_ = rc.Close()
	}
	if got, want := tt.bodies.Load(), int32(1); got != want {
		t.Fatalf("expected %d wrapped body, got %d", want, got)
	}
	// Exactly one close: catches both leaks (0) and double-close regressions (>1).
	if got := tt.closed.Load(); got != 1 {
		t.Errorf("response body close count = %d, want 1, on non-2xx GetAttachment path", got)
	}
}

// TestGetAttachment2xxBodyIsCallersResponsibility verifies the success path
// hands the (still-open) body back to the caller.
func TestGetAttachment2xxBodyIsCallersResponsibility(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ciphertext-bytes"))
	}))
	defer srv.Close()

	tt := &trackingTransport{inner: http.DefaultTransport}
	c, err := NewClient(
		WithBaseURL(srv.URL),
		WithAppVersion("test/0.0.1"),
		WithHTTPClient(&http.Client{Transport: tt}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	rc, err := c.GetAttachment(context.Background(), "att-id")
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	// Body should NOT yet be closed — caller owns it.
	if got := tt.closed.Load(); got != 0 {
		t.Errorf("body closed prematurely on 2xx (closed=%d)", got)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "ciphertext-bytes" {
		t.Errorf("body = %q, want %q", got, "ciphertext-bytes")
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := tt.closed.Load(); got != 1 {
		t.Errorf("body close count = %d, want 1", got)
	}
}

// TestDoJSONClosesBodyOnAllPaths uses trackingTransport to verify that
// doJSON closes the body on success, on non-2xx, and on JSON decode error.
func TestDoJSONClosesBodyOnAllPaths(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		respObj interface{}
		wantErr bool
	}{
		{
			name: "200 valid JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				_, _ = w.Write([]byte(`{"X":1}`))
			},
			respObj: &struct{ X int }{},
			wantErr: false,
		},
		{
			name: "404 HTML body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
				_, _ = w.Write([]byte("<html/>"))
			},
			respObj: &struct{ X int }{},
			wantErr: true,
		},
		{
			name: "200 malformed JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				_, _ = w.Write([]byte("not-json"))
			},
			respObj: &struct{ X int }{},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			tt := &trackingTransport{inner: http.DefaultTransport}
			c, err := NewClient(
				WithBaseURL(srv.URL),
				WithAppVersion("test/0.0.1"),
				WithHTTPClient(&http.Client{Transport: tt}),
			)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			req, err := c.newRequest(context.Background(), http.MethodGet, "/anything", nil)
			if err != nil {
				t.Fatalf("newRequest: %v", err)
			}
			err = c.doJSON(context.Background(), req, tc.respObj)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, want := tt.bodies.Load(), int32(1); got != want {
				t.Fatalf("expected %d wrapped body, got %d", want, got)
			}
			// Exactly one close: catches leaks (0) and double-close regressions (>1).
			if got := tt.closed.Load(); got != 1 {
				t.Errorf("body close count = %d, want 1", got)
			}
		})
	}
}
