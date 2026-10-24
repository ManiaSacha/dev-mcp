# Contributing

Thanks for helping. Bug reports, fixes and new redaction patterns are all welcome.

## Before you start

- For anything larger than a small fix, open an issue first so we can agree on the approach.
- For security problems, follow [SECURITY.md](SECURITY.md) instead of opening an issue.

## Setup

You need Go 1.27 or newer. `go.mod` selects the exact toolchain, and Go downloads it for you.

```bash
git clone https://github.com/ManiaSacha/dev-mcp.git
cd dev-mcp
go build -o dev-mcp ./cmd/dev-mcp
```

## Checks to run before a pull request

```bash
gofmt -l .
go vet ./...
go test -race ./...
golangci-lint run ./...
```

- `go test -race` needs a C compiler on Linux and Windows (GCC or MinGW-w64).
- CI uses golangci-lint v2.15.0 and `govulncheck`. Use the same version of golangci-lint locally.
- CI runs the tests on Linux, macOS and Windows. Linux must pass. The macOS and Windows jobs report problems without blocking a merge, because some tests do not yet pass there. Fixes for that are welcome.

## Code rules

These keep the tools safe, so a change that breaks one will not be merged:

- **No shell.** Run programs with an argument vector (`exec.CommandContext`), never `sh -c`. Reject caller-supplied values that start with `-` before they reach a program.
- **Redact first.** Any text that comes from a log, a container or a database and goes back to the client must pass through `internal/redaction`. Redact before truncating.
- **Paths go through `internal/pathguard`.** Any tool that reads a caller-supplied path must call `pathguard.Check`.
- **Read-only by default.** New tools should not write, start or stop anything unless that is their purpose and the README says so.
- **stdout is for JSON-RPC only.** Log to stderr.
- Put code shared by several tool packages in `internal/tools/toolutil`.
- Match the style around your change: naming, comment density and idiom.

## Tests

Every behaviour change needs a test. Use synthetic fixtures only. Never put a real token, key or password in a test, even an expired one. Fake values such as `hunter2` or `ghp_` followed by repeated letters are fine.

## Pull requests

- Keep each pull request to one change.
- Describe the problem, the fix and how you checked it.
- Make sure CI is green on the `test`, `lint` and `vulncheck` jobs.
