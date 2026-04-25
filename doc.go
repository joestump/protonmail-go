// Package protonmail is a Go client for the (unofficial) Proton Mail API.
//
// # Status
//
// This is a v0.x library. The API will churn until v1.0.0; pin to a
// specific tag in production. The Proton Mail API itself is unofficial,
// reverse-engineered, and unstable; see the project README for the
// stability disclaimer.
//
// # Quickstart
//
//	package main
//
//	import (
//	    "context"
//	    "log"
//
//	    "github.com/joestump/protonmail-go"
//	)
//
//	func main() {
//	    c, err := protonmail.NewClient(
//	        protonmail.WithAppVersion("Other"),
//	    )
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    if _, err := c.AuthInfo(context.Background(), "user@example.com"); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// # Authentication
//
// Authentication is a multi-step SRP flow: AuthInfo, Auth, optionally
// AuthTOTP, then ListKeySalts and Unlock. A higher-level Login wrapper
// is planned (see issue #11).
//
// # Cancellation
//
// Every networked method takes a context.Context as its first parameter.
// Cancellation propagates to the underlying HTTP request.
//
// # Errors
//
// Errors returned from networked methods can be inspected with
// errors.Is and errors.As against the package's typed errors and
// sentinels (ErrUnauthorized, ErrNotFound, ErrRateLimited, etc).
//
// For non-2xx responses the client returns one of two typed errors:
//
//   - *APIError when the response body parses as a Proton error envelope
//     (the common case). It carries Proton's application-layer Code in
//     addition to the message.
//   - *HTTPError when the response body does not parse as a Proton error
//     envelope (rare — for example, an HTML error page from a misconfigured
//     proxy or upstream gateway). It carries the HTTP status code.
//
// Both wrap the same set of sentinels, so errors.Is(err, ErrUnauthorized)
// (and friends) works uniformly regardless of which concrete type the
// caller received.
//
// # Logging
//
// The Client uses *slog.Logger for diagnostic output. Pass
// WithLogger(slog.New(slog.DiscardHandler)) to silence (default), or
// a debug-level logger to see request/response traces. Sensitive headers
// and bodies (tokens, refresh tokens, SRP proofs) are not logged.
package protonmail
