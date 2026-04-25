# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Note: pre-1.0 releases may break the API at any minor bump.

## [Unreleased]

### Changed
- **BREAKING:** Every networked method on `*Client` and `*Importer` now takes
  `ctx context.Context` as the first parameter (#2). Existing callers must
  update their call sites.
- **BREAKING:** The `Client.ReAuth` callback signature changed from
  `func() error` to `func(context.Context) error` (#2).
- **BREAKING:** `Client` struct fields are now unexported. Use
  `NewClient(opts ...Option)` instead of struct-literal initialization (#5).
- **BREAKING:** `Client.RootURL` removed in favor of the `WithBaseURL` option.
  HTTPS is required; an `http://` carve-out exists only for loopback hosts so
  tests can target `httptest.Server` (#5).
- **BREAKING:** `Client.AppVersion` removed in favor of `WithAppVersion`. App
  version is required at construction; `NewClient` returns an error otherwise
  (#5).
- **BREAKING:** `Client.HTTPClient` removed in favor of the `WithHTTPClient`
  option (#5).
- **BREAKING:** `Client.ReAuth` field removed in favor of the `WithReAuth`
  option (#5).
- **BREAKING:** `Client.Debug` removed; replaced by `WithLogger(*slog.Logger)`.
  Verbose output is gated by log level on the supplied logger; coordinates
  with #6 (#5).
- **BREAKING:** `Client.Debug` field removed; replace with
  `WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))`
  for equivalent debug output (#6).
- Removed: `import "log"` — all logging now flows through `*slog.Logger`
  (#6).
- Default User-Agent now identifies the library
  (`protonmail-go/<version>`) instead of impersonating Firefox (#5, #D5).
- Improved: `do` now consistently closes the response body on all error
  paths and returns `(nil, err)` on transport errors (#4).
- **BREAKING:** `APIError`'s `Error()` and field accessors may have changed;
  the type now implements `Is(target error) bool` (#3).
- Errors returned from networked methods now wrap their cause with `%w`. Use
  `errors.Is`/`errors.As` (#3).
- **BREAKING:** Some errors that were string-formatted are now typed:
  `*HTTPError` for non-2xx HTTP failures (#3).
- New sentinels exported: `ErrUnauthorized`, `ErrNotFound`, `ErrRateLimited`,
  `ErrNoUnlockableKeys`, `ErrImporterClosed` (#3).
- `*APIError` now carries an optional `HTTPError *HTTPError` field and exposes
  `Unwrap()` so `errors.Is(err, ErrRateLimited)` / `ErrNotFound` resolve via
  HTTP status when Proton's application "Code" is unknown. Proton "Code" is
  treated as a hint; HTTP status is the source of truth (#3).
- `(*APIError).Error()` and `(*HTTPError).Error()` are now nil-safe and return
  `"<nil>"` instead of panicking on a nil receiver (#3).
- `HTTPError.Body` is documented as potentially containing sensitive data
  (tokens, addresses); callers must not log it verbatim (#3).

### Fixed
- `doJSON` now checks HTTP status before JSON-decoding (was producing
  confusing decode errors on non-JSON 4xx/5xx responses). API-error-shaped
  bodies still surface as `*APIError`; everything else returns a
  status-bearing error (#4).
- `GetAttachment` no longer leaks the response body on non-2xx responses;
  the body is drained and closed before the error is returned (#4).
- Debug logging no longer includes raw `Authorization` headers,
  `AccessToken`, `RefreshToken`, `ClientProof`, or `ServerProof` values.
  Sensitive headers are redacted via an internal helper; request and
  response bodies are no longer dumped (#6, security follow-up).

## [0.1.0] - TBD

- Initial extraction from emersion/hydroxide.
