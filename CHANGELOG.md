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

## [0.1.0] - TBD

- Initial extraction from emersion/hydroxide.
