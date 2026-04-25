package protonmail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestAPIError_Is_Unauthorized(t *testing.T) {
	cases := []struct {
		name string
		code int
		want bool
	}{
		{"http 401 fallback", http.StatusUnauthorized, true},
		{"proton invalid creds 8002", 8002, true},
		{"proton 1000 success", 1000, false},
		{"proton 2501 not found", 2501, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &APIError{Code: tc.code, Message: "denied"}
			got := errors.Is(err, ErrUnauthorized)
			if got != tc.want {
				t.Fatalf("errors.Is(APIError{Code:%d}, ErrUnauthorized) = %v; want %v", tc.code, got, tc.want)
			}
		})
	}
}

// TestAPIError_Is_NotFound_ViaHTTPError verifies that ErrNotFound is matched
// through the *APIError -> *HTTPError unwrap chain (HTTP 404), not via any
// guess at a Proton application code.
func TestAPIError_Is_NotFound_ViaHTTPError(t *testing.T) {
	err := &APIError{
		Code:    9999,
		Message: "missing",
		HTTPError: &HTTPError{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
		},
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected APIError wrapping HTTP 404 to match ErrNotFound")
	}

	// Without an HTTPError, an unknown Proton code does NOT match ErrNotFound.
	bare := &APIError{Code: 2501, Message: "missing"}
	if errors.Is(bare, ErrNotFound) {
		t.Fatalf("APIError.Code=2501 alone (no HTTPError) should not match ErrNotFound")
	}
}

// TestAPIError_Is_RateLimited_ViaHTTPError verifies that ErrRateLimited is
// matched through the unwrap chain (HTTP 429).
func TestAPIError_Is_RateLimited_ViaHTTPError(t *testing.T) {
	err := &APIError{
		Code:    1234,
		Message: "slow down",
		HTTPError: &HTTPError{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
		},
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected APIError wrapping HTTP 429 to match ErrRateLimited")
	}

	bare := &APIError{Code: 429, Message: "slow"}
	if errors.Is(bare, ErrRateLimited) {
		t.Fatalf("APIError.Code=429 alone (no HTTPError) should not match ErrRateLimited")
	}
}

// TestAPIError_Unwrap exercises the explicit Unwrap() method.
func TestAPIError_Unwrap(t *testing.T) {
	hErr := &HTTPError{StatusCode: 503, Status: "503 Service Unavailable"}
	err := &APIError{Code: 1, Message: "x", HTTPError: hErr}
	if errors.Unwrap(err) != hErr {
		t.Fatalf("APIError.Unwrap() did not return wrapped HTTPError")
	}

	bare := &APIError{Code: 1, Message: "x"}
	if errors.Unwrap(bare) != nil {
		t.Fatalf("APIError.Unwrap() with nil HTTPError should return nil")
	}

	var nilErr *APIError
	if errors.Unwrap(nilErr) != nil {
		t.Fatalf("nil *APIError Unwrap should return nil")
	}
}

func TestAPIError_As(t *testing.T) {
	wrapped := fmt.Errorf("call failed: %w", &APIError{Code: 8002, Message: "bad password"})
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatalf("expected errors.As to extract APIError")
	}
	if apiErr.Code != 8002 || apiErr.Message != "bad password" {
		t.Fatalf("unexpected APIError fields: %+v", apiErr)
	}
}

// TestAPIError_AsHTTPError_ThroughChain verifies errors.As can dig through
// APIError to HTTPError via Unwrap.
func TestAPIError_AsHTTPError_ThroughChain(t *testing.T) {
	err := &APIError{
		Code:      8002,
		Message:   "bad creds",
		HTTPError: &HTTPError{StatusCode: 401, Status: "401 Unauthorized", Body: []byte("nope")},
	}
	wrapped := fmt.Errorf("auth: %w", err)
	var hErr *HTTPError
	if !errors.As(wrapped, &hErr) {
		t.Fatalf("expected errors.As to extract *HTTPError through APIError")
	}
	if hErr.StatusCode != 401 {
		t.Fatalf("HTTPError.StatusCode = %d, want 401", hErr.StatusCode)
	}
}

func TestAPIError_UnwrapsThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("auth: %w", &APIError{Code: 401, Message: "no"})
	if !errors.Is(wrapped, ErrUnauthorized) {
		t.Fatalf("expected wrapped APIError(401) to unwrap to ErrUnauthorized")
	}
}

func TestHTTPError_Error(t *testing.T) {
	e := &HTTPError{StatusCode: 503, Status: "503 Service Unavailable"}
	want := "protonmail: HTTP 503 Service Unavailable"
	if got := e.Error(); got != want {
		t.Fatalf("HTTPError.Error() = %q; want %q", got, want)
	}
	e2 := &HTTPError{StatusCode: 500}
	if got := e2.Error(); got != "protonmail: HTTP 500" {
		t.Fatalf("HTTPError.Error() fallback = %q", got)
	}
}

// TestHTTPError_NilReceiver verifies (*HTTPError).Error() does not panic on
// a nil receiver. Matches the nil-safety of (*HTTPError).Is.
func TestHTTPError_NilReceiver(t *testing.T) {
	var e *HTTPError
	if got := e.Error(); got != "<nil>" {
		t.Fatalf("nil *HTTPError Error() = %q, want \"<nil>\"", got)
	}
	if e.Is(ErrUnauthorized) {
		t.Fatalf("nil *HTTPError should not match any sentinel")
	}
}

func TestHTTPError_Is(t *testing.T) {
	cases := []struct {
		status int
		target error
		want   bool
	}{
		{401, ErrUnauthorized, true},
		{404, ErrNotFound, true},
		{429, ErrRateLimited, true},
		{500, ErrUnauthorized, false},
		{500, ErrNotFound, false},
		{500, ErrRateLimited, false},
	}
	for _, tc := range cases {
		e := &HTTPError{StatusCode: tc.status, Status: http.StatusText(tc.status)}
		if got := errors.Is(e, tc.target); got != tc.want {
			t.Fatalf("errors.Is(HTTPError{%d}, %v) = %v; want %v", tc.status, tc.target, got, tc.want)
		}
	}
}

func TestHTTPError_AsThroughWrapping(t *testing.T) {
	src := &HTTPError{StatusCode: 502, Status: "502 Bad Gateway", Body: []byte("oops")}
	wrapped := fmt.Errorf("attachments: %w", src)
	var got *HTTPError
	if !errors.As(wrapped, &got) {
		t.Fatalf("expected errors.As to extract *HTTPError from wrapped error")
	}
	if got.StatusCode != 502 || string(got.Body) != "oops" {
		t.Fatalf("unexpected HTTPError contents: %+v", got)
	}
}

func TestSentinel_ErrNoUnlockableKeys_Unwraps(t *testing.T) {
	wrapped := fmt.Errorf("auth: %w", ErrNoUnlockableKeys)
	if !errors.Is(wrapped, ErrNoUnlockableKeys) {
		t.Fatalf("expected wrapped ErrNoUnlockableKeys to unwrap")
	}
}

func TestSentinel_ErrImporterClosed_Identity(t *testing.T) {
	if !errors.Is(ErrImporterClosed, ErrImporterClosed) {
		t.Fatalf("ErrImporterClosed should equal itself")
	}
	wrapped := fmt.Errorf("commit: %w", ErrImporterClosed)
	if !errors.Is(wrapped, ErrImporterClosed) {
		t.Fatalf("expected wrapped ErrImporterClosed to unwrap")
	}
}

func TestSentinel_DistinctIdentities(t *testing.T) {
	if errors.Is(ErrUnauthorized, ErrNotFound) {
		t.Fatalf("ErrUnauthorized should not match ErrNotFound")
	}
	if errors.Is(ErrNoUnlockableKeys, ErrImporterClosed) {
		t.Fatalf("ErrNoUnlockableKeys should not match ErrImporterClosed")
	}
}

func TestAPIError_NilReceiver(t *testing.T) {
	var err *APIError
	// Calling Is on nil should not panic and should return false.
	if err.Is(ErrUnauthorized) {
		t.Fatalf("nil *APIError should not match any sentinel")
	}
	// Calling Error() on nil should not panic.
	if got := err.Error(); got != "<nil>" {
		t.Fatalf("nil *APIError Error() = %q, want \"<nil>\"", got)
	}
}

// TestErrorsIs_EndToEnd_Unauthorized exercises the full networked path:
// httptest server returns 401 with a Proton envelope, c.AuthInfo surfaces
// the error, and errors.Is(err, ErrUnauthorized) reports true. This is the
// contract callers actually use — type-level tests above are not enough.
func TestErrorsIs_EndToEnd_Unauthorized(t *testing.T) {
	_, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"Code": 8002, "Error": "Invalid credentials"}`)
	})
	defer cleanup()

	_, err := c.AuthInfo(context.Background(), "nobody@example.com")
	if err == nil {
		t.Fatal("expected error from 401 Proton envelope, got nil")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("errors.Is(err, ErrUnauthorized) = false; want true (err=%v, type=%T)", err, err)
	}
}

// TestErrorsIs_EndToEnd_InvalidCreds_AsCode covers the case where the server
// replies with HTTP 200 but a Proton "invalid credentials" envelope code.
// (Some Proton endpoints historically returned 2xx with an error code.)
func TestErrorsIs_EndToEnd_InvalidCreds_AsCode(t *testing.T) {
	_, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Code": 8002, "Error": "Invalid credentials"}`)
	})
	defer cleanup()

	_, err := c.AuthInfo(context.Background(), "nobody@example.com")
	if err == nil {
		t.Fatal("expected error from 8002 envelope, got nil")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("errors.Is(err, ErrUnauthorized) = false; want true (err=%v, type=%T)", err, err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed")
	}
	if apiErr.Code != 8002 {
		t.Fatalf("APIError.Code = %d, want 8002", apiErr.Code)
	}
}
