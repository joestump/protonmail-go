package protonmail

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors that callers can match with errors.Is.
//
// These represent stable categories of failure that are useful to branch on
// at the call site. Each is wrapped in more specific context where it is
// returned, so callers should prefer errors.Is(err, ErrX) over equality.
var (
	// ErrUnauthorized indicates the request was rejected because the caller
	// is not authenticated (or the access token has expired and could not be
	// refreshed). Returned for HTTP 401 and the equivalent Proton API codes.
	ErrUnauthorized = errors.New("protonmail: unauthorized")

	// ErrNotFound indicates the requested resource does not exist. Returned
	// for HTTP 404 and the equivalent Proton API codes.
	ErrNotFound = errors.New("protonmail: not found")

	// ErrRateLimited indicates the caller has exceeded a rate limit. Returned
	// for HTTP 429 and the equivalent Proton API codes.
	ErrRateLimited = errors.New("protonmail: rate limited")

	// ErrNoUnlockableKeys indicates none of the user or address keys could be
	// decrypted with the supplied passphrase. Returned from key-unlock paths.
	ErrNoUnlockableKeys = errors.New("protonmail: no unlockable keys")

	// ErrImporterClosed indicates an operation was attempted on an Importer
	// that has already been closed.
	ErrImporterClosed = errors.New("protonmail: importer closed")
)

// Proton API "Code" values that map to sentinels. Proton returns its own
// numeric code on top of the HTTP status — these are the ones we recognise.
//
// Note: Proton's documented "application" codes are 4-digit integers carried
// in the response envelope's "Code" field; they are NOT HTTP status codes.
// Where we list HTTP status numbers below (e.g. 401), it is a defensive
// fallback against servers or proxies that put the HTTP status in the
// envelope code. The source of truth for HTTP-level categorisation
// (rate-limit, not-found) is *HTTPError, reached via errors.Unwrap.
const (
	// apiCodeInvalidCredentials is Proton's documented code for a wrong
	// username/password combination on the auth endpoints.
	apiCodeInvalidCredentials = 8002

	// apiCodeAuthRequired is a defensive fallback: some servers/proxies put
	// the HTTP 401 in the envelope's "Code" instead of (or in addition to)
	// a Proton 4-digit code. Match on it so users aren't bitten by that.
	apiCodeAuthRequired = http.StatusUnauthorized
)

// Is reports whether the APIError matches one of the package sentinels.
//
// APIError carries Proton's application-layer "Code" field, which is a hint:
// the source of truth for HTTP-level categorisation (rate-limited, not-found)
// is the wrapped *HTTPError, reachable via errors.Unwrap. errors.Is walks the
// chain, so a caller doing errors.Is(err, ErrRateLimited) will succeed via
// HTTPError.Is even if APIError.Is does not match.
//
// Currently APIError only matches ErrUnauthorized directly, on the documented
// Proton code 8002 ("invalid credentials") and the defensive HTTP-401 fallback.
// Other sentinels (ErrNotFound, ErrRateLimited) are left to *HTTPError.
func (err *APIError) Is(target error) bool {
	if err == nil || target == nil {
		return false
	}
	if target == ErrUnauthorized {
		return err.Code == apiCodeInvalidCredentials ||
			err.Code == apiCodeAuthRequired
	}
	return false
}

// Unwrap returns the underlying *HTTPError, if any. This lets errors.Is and
// errors.As walk from an APIError to the HTTP-level error so callers can
// match on HTTP status (e.g. ErrRateLimited via HTTP 429) even when Proton's
// application "Code" is unknown.
func (err *APIError) Unwrap() error {
	if err == nil || err.HTTPError == nil {
		return nil
	}
	return err.HTTPError
}

// HTTPError represents an HTTP-level failure: a non-2xx response that did not
// carry a parseable APIError body (or that the caller wants to surface as a
// raw HTTP failure).
type HTTPError struct {
	StatusCode int
	Status     string

	// Body is the raw response body (truncated to a sane upper bound by the
	// caller). It MAY contain sensitive data — tokens, email addresses,
	// nonces, partial PGP material — depending on the endpoint. Do NOT log
	// Body verbatim; surface it only to the user, in error messages they
	// have explicitly opted into.
	Body []byte
}

// Error implements error. It is nil-safe: calling Error() on a nil receiver
// returns "<nil>" instead of panicking.
func (e *HTTPError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Status != "" {
		return fmt.Sprintf("protonmail: HTTP %s", e.Status)
	}
	return fmt.Sprintf("protonmail: HTTP %d", e.StatusCode)
}

// Is reports whether the HTTPError matches one of the package sentinels,
// based on the HTTP status code.
func (e *HTTPError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	switch target {
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrRateLimited:
		return e.StatusCode == http.StatusTooManyRequests
	}
	return false
}
