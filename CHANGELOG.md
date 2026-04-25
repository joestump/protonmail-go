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
- Default User-Agent now identifies the library
  (`protonmail-go/<version>`) instead of impersonating Firefox (#5, #D5).

## [0.1.0] - TBD

- Initial extraction from emersion/hydroxide.
