# protonmail-go

[![CI](https://github.com/joestump/protonmail-go/actions/workflows/ci.yml/badge.svg)](https://github.com/joestump/protonmail-go/actions/workflows/ci.yml)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/joestump/protonmail-go.svg)](https://pkg.go.dev/github.com/joestump/protonmail-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/joestump/protonmail-go)](https://goreportcard.com/report/github.com/joestump/protonmail-go)
[![License](https://img.shields.io/github/license/joestump/protonmail-go)](LICENSE)

A Go client library for the Proton Mail API.

> **Status: experimental.** This is a `v0.x` library. The public API will
> churn as the package is refactored and tested. Once tags exist, pin to
> a specific version and read the changelog before upgrading; until then,
> track `main` at your own risk. Don't use this in anything
> mission-critical until `v1.0.0`. Semantic versioning kicks in at `v1`;
> until then, every minor bump may break.

## What this is

`protonmail-go` is a standalone Go client for the unofficial Proton Mail
HTTP API. It was extracted from the `protonmail/` directory of
[emersion/hydroxide](https://github.com/emersion/hydroxide) so the API
client can be reused outside the hydroxide bridge — for tools, bots,
backup utilities, custom clients, and so on.

The package name is `protonmail`. The module path is
`github.com/joestump/protonmail-go`.

## What this is not

- **Not an SMTP/IMAP bridge.** If you want to use Proton Mail with a
  standard mail client (Thunderbird, mutt, etc.), use
  [emersion/hydroxide](https://github.com/emersion/hydroxide) — that's
  exactly what it does, and it does it well.
- **Not an official Proton SDK.** Proton does not publish or endorse a
  Go client. This library talks to the same HTTP endpoints that the
  Proton web and desktop apps use, based on reverse-engineering work
  done in hydroxide.
- **Not a stable contract with Proton.** See
  [API stability](#api-stability) below.

## Install

```sh
go get github.com/joestump/protonmail-go
```

Requires Go 1.25 or later.

## Quickstart

The snippet below targets the `v0.1.x` API as it exists today. It does
not log in — it just fetches the SRP auth info for an address as a
minimal round-trip against the API.

```go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/joestump/protonmail-go"
)

func main() {
    c := &protonmail.Client{
        RootURL:    "https://api.proton.me",
        AppVersion: "Other",
        HTTPClient: http.DefaultClient,
    }
    if _, err := c.AuthInfo("user@example.com"); err != nil {
        log.Fatal(err)
    }
    fmt.Println("auth info OK")
}
```

> **Heads up:** the auth surface changes substantially in `v0.2` — a
> `NewClient` constructor with options and a higher-level `Login` flow
> are planned. The shape above is the current `v0.1.x` API only.

Runnable examples are coming to an `examples/` directory in a follow-up
release. Until then, see the godoc on
[pkg.go.dev](https://pkg.go.dev/github.com/joestump/protonmail-go) and
the original consumer in
[hydroxide](https://github.com/emersion/hydroxide) for reference usage.

## Feature matrix

Every endpoint below was lifted from hydroxide as-is. "Implemented"
means the code is in this repo and compiles. "Tested" means unit/integration
tests cover it in this repo. "Verified against current Proton API" means
someone has run it against `api.proton.me` recently and confirmed it
works. The honest answer for everything today is: implemented, untested,
unverified.

| Area                  | Implemented | Tested | Verified against current Proton API |
| --------------------- | :---------: | :----: | :---------------------------------: |
| Auth (SRP, 2FA)       |      ✓      |   ✗    |                  ⚠                  |
| Messages              |      ✓      |   ✗    |                  ⚠                  |
| Conversations         |      ✓      |   ✗    |                  ⚠                  |
| Contacts              |      ✓      |   ✗    |                  ⚠                  |
| Labels                |      ✓      |   ✗    |                  ⚠                  |
| Addresses             |      ✓      |   ✗    |                  ⚠                  |
| Keys                  |      ✓      |   ✗    |                  ⚠                  |
| Attachments           |      ✓      |   ✗    |                  ⚠                  |
| Events                |      ✓      |   ✗    |                  ⚠                  |
| Calendar              |      ✓      |   ✗    |                  ⚠                  |
| Import                |      ✓      |   ✗    |                  ⚠                  |

A test scaffold and CI are being added in parallel. The matrix will be
updated as coverage lands. Treat any cell that isn't "verified" as
liable to silently break the next time Proton ships a web update.

## API stability

The Proton Mail API is **unofficial and reverse-engineered**. Proton
does not document it, does not publish a stability contract, and is
free to change request shapes, response shapes, error codes, auth
flows, or rate limits at any time without notice. Web/desktop client
updates routinely move things around.

Practically, that means:

- A release of this library that worked yesterday may stop working
  today if Proton ships a change.
- This library's own API (function signatures, struct fields) is also
  in flux until `v1.0.0` — see the status callout at the top.
- If you depend on this in a service, vendor it, pin it, and have a
  plan for what happens when an endpoint changes shape.

If a request stops working, please open an issue with the failing
endpoint, the request body, and the Proton response — that's the
fastest path to a fix.

## Documentation

- API reference:
  [pkg.go.dev/github.com/joestump/protonmail-go](https://pkg.go.dev/github.com/joestump/protonmail-go)
- Source: this repository
- Reference consumer: [emersion/hydroxide](https://github.com/emersion/hydroxide)

## Contributing

Issues and pull requests are welcome. A `CONTRIBUTING.md` with
specifics (branch naming, PR conventions, test expectations) is on the
way; until then, the short version is:

- Open an issue before doing anything large, so we can agree on shape.
- Run `go fmt`, `go vet`, and (when CI lands) `make lint` before
  submitting.
- Keep PRs small and focused; one logical change per PR.

## Attribution

This package is a fork-and-extract of the `protonmail/` directory from
[emersion/hydroxide](https://github.com/emersion/hydroxide), which is
MIT-licensed and copyright © 2017 emersion. The original copyright and
license are preserved in [LICENSE](LICENSE). Enormous thanks to
[@emersion](https://github.com/emersion) and the hydroxide contributors
— this library would not exist without their reverse-engineering work.

## License

[MIT](LICENSE) — copyright © 2017 emersion, with extraction and
ongoing maintenance by Joe Stump and contributors.
