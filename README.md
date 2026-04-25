# protonmail-go

[![pkg.go.dev](https://pkg.go.dev/badge/github.com/joestump/protonmail-go.svg)](https://pkg.go.dev/github.com/joestump/protonmail-go)
[![License](https://img.shields.io/github/license/joestump/protonmail-go)](LICENSE)

> **⚠️ Archived study artifact. For production Proton Mail integration in
> Go, use [`github.com/ProtonMail/go-proton-api`](https://github.com/ProtonMail/go-proton-api),
> the official client maintained by Proton AG.**

## What this is

A standalone, modernized extraction of the `protonmail/` directory from
[emersion/hydroxide](https://github.com/emersion/hydroxide), released as
`v0.1.0` and frozen. The goal was to lift hydroxide's reverse-engineered
Proton Mail HTTP client out of the bridge and apply a focused
Go-modernization sprint:

- `NewClient(opts ...Option)` constructor with functional options
- `context.Context` plumbed through every networked method
- Typed errors (`*APIError`, `*HTTPError`) and sentinels (`ErrUnauthorized`,
  `ErrNotFound`, `ErrRateLimited`, `ErrNoUnlockableKeys`,
  `ErrImporterClosed`) for `errors.Is` / `errors.As`
- Injectable `*slog.Logger` with secret redaction (no more raw token
  bodies in debug output)
- HTTPS-only `WithBaseURL` with a documented loopback carve-out for tests
- Test scaffolding: `httptest` fixtures, integration build tag, coverage
  on pure functions
- Package `doc.go` and godoc on every exported symbol

The package compiles, has tests, and represents an honest study of what
hydroxide's API client looks like with modern Go ergonomics applied.

## Why it's archived

The Proton API surface in this library is the one hydroxide reverse-engineered
in 2017, with limited drift updates since. While building `v0.2`, it became
clear that `github.com/ProtonMail/go-proton-api` (Proton AG's official Go
client, used by Proton Bridge) covers the same surface — with current API
endpoints, authoritative Proton error codes (`HumanVerificationRequired = 9001`,
etc.), built-in `Retry-After` handling, FIDO2 support, and ongoing maintenance.

Maintaining a parallel client to track Proton's API drift is not a useful
use of effort when Proton publishes one themselves. So:

- Production Proton work in Go → use [`github.com/ProtonMail/go-proton-api`](https://github.com/ProtonMail/go-proton-api).
- This repo → preserved at `v0.1.0` as a smaller, audit-friendly,
  resty-free study of the Proton API surface, useful as a reference or
  for very narrow embedded use cases.

## What's not in this library (and won't be)

- Current Proton API path versions (`/auth/v4`, `/mail/v4/`, `/core/v4/`,
  `/contacts/v4/`).
- CAPTCHA / human-verification (`x-pm-human-verification-token`) support.
- FIDO2 / WebAuthn 2FA.
- Proton Drive, Proton Calendar (full surface), Key Transparency, and
  modern event-stream types.
- 429/503 retry with `Retry-After` parsing.
- Refresh-token race serialization.

All of the above are present and working in `go-proton-api`.

## Install (if you really want this specific package)

```sh
go get github.com/joestump/protonmail-go@v0.1.0
```

Requires Go 1.25 or later.

## Quickstart

```go
package main

import (
    "context"
    "log"

    "github.com/joestump/protonmail-go"
)

func main() {
    c, err := protonmail.NewClient(
        protonmail.WithAppVersion("Other"),
    )
    if err != nil {
        log.Fatal(err)
    }
    if _, err := c.AuthInfo(context.Background(), "user@example.com"); err != nil {
        log.Fatal(err)
    }
    log.Println("auth info OK")
}
```

See [`pkg.go.dev`](https://pkg.go.dev/github.com/joestump/protonmail-go)
for the full API reference.

## Attribution

This package is a fork-and-extract of the `protonmail/` directory from
[emersion/hydroxide](https://github.com/emersion/hydroxide), which is
MIT-licensed and copyright © 2017 emersion. The original copyright and
license are preserved in [LICENSE](LICENSE). Enormous thanks to
[@emersion](https://github.com/emersion) and the hydroxide contributors
— this package would not exist without their reverse-engineering work.

## License

[MIT](LICENSE) — copyright © 2017 emersion, with extraction and
modernization by Joe Stump.
