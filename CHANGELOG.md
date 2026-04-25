# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-04-25

This is the first and final release of `protonmail-go`. The repository is
archived as a study artifact. For production Proton Mail integration in
Go, use [`github.com/ProtonMail/go-proton-api`](https://github.com/ProtonMail/go-proton-api),
the official client maintained by Proton AG and used by Proton Bridge.

### Added (relative to the lift-and-shift extraction from hydroxide's `protonmail/`)
- Package `doc.go` and godoc comments on every exported symbol (#9).
- Test scaffolding: `httptest` fixtures, integration build tag, unit tests
  for SRP / password / `Timestamp` / `EventMessage` / `APIError`,
  end-to-end coverage of the 401 ReAuth retry path (#7).
- `*HTTPError` type and exported sentinel errors: `ErrUnauthorized`,
  `ErrNotFound`, `ErrRateLimited`, `ErrNoUnlockableKeys`,
  `ErrImporterClosed` (#3).
- `NewClient(opts ...Option) (*Client, error)` constructor with
  `WithBaseURL`, `WithHTTPClient`, `WithAppVersion`, `WithUserAgent`,
  `WithReAuth`, `WithLogger` options (#5).
- Injectable `*slog.Logger` with structured-attr debug output and an
  internal `redactedHeaders` helper for `Authorization` / `X-Pm-Uid` (#6).
- GitHub Actions CI: build / vet / test / staticcheck / govulncheck on
  Linux/macOS/Windows × Go 1.25 (#8).
- Dependabot configuration for `gomod` and `github-actions` (#8).
- README expanded into a full pkg.go.dev landing page (#10).

### Changed
- Every networked method on `*Client` and `*Importer` takes
  `ctx context.Context` as the first parameter (#2). The `ReAuth`
  callback signature is now `func(context.Context) error`.
- `Client` struct fields are unexported. The struct must be constructed
  via `NewClient(opts ...Option)` — zero-value `Client{}` is no longer
  reachable from outside the package (#5).
- `WithBaseURL` requires `https://`. `http://` is accepted only for
  loopback hosts (`127.0.0.1`, `::1`, `localhost`) so tests can target
  `httptest.Server` (#5).
- App version (`x-pm-appversion`) is required; `NewClient` returns an
  error if `WithAppVersion` is not provided (#5).
- Default User-Agent identifies the library
  (`protonmail-go/<version>`) instead of impersonating Firefox (#5).
- `do` and `doJSON` consistently close the response body on all paths;
  `doJSON` checks HTTP status before JSON-decoding so non-JSON 4xx/5xx
  responses surface as a status-bearing error rather than a confusing
  decode error (#4).
- All error-wrapping `fmt.Errorf` calls use `%w`. `*APIError` implements
  `Is(target error) bool` and `Unwrap()` returning an underlying
  `*HTTPError`, so `errors.Is(err, ErrRateLimited)` /
  `ErrNotFound` / `ErrUnauthorized` resolve via HTTP status when
  Proton's application "Code" is unknown (#3).
- `(*APIError).Error()` and `(*HTTPError).Error()` are nil-safe (#3).
- All package-level `log` calls replaced with `c.logger.{Debug,Warn}`.
  Request and response bodies are no longer dumped in debug output;
  raw `AccessToken` / `RefreshToken` / `ClientProof` / `ServerProof` /
  `Authorization` values no longer reach any sink (#6).
- `GetAttachment` drains and closes the response body on non-2xx
  before returning the error (was leaking connections) (#4).

### Removed
- `Client.RootURL`, `Client.AppVersion`, `Client.HTTPClient`,
  `Client.ReAuth`, `Client.Debug` exported fields (#5).
- The hardcoded Firefox `User-Agent` string (#5).
- Direct `import "log"` from production code (#6).
- The deprecated `io/ioutil` import (#1).

### Notes
- This release does not migrate to current Proton API path versions
  (`/auth/v4`, `/mail/v4/`, `/core/v4/`, `/contacts/v4/`), nor does it
  add CAPTCHA / human-verification, FIDO2 / WebAuthn 2FA, or
  `Retry-After` retry. Those gaps were intentionally left in place;
  callers needing current Proton API behavior should use
  [`github.com/ProtonMail/go-proton-api`](https://github.com/ProtonMail/go-proton-api).

[0.1.0]: https://github.com/joestump/protonmail-go/releases/tag/v0.1.0
