# dev-mcp

`dev-mcp` is a single-binary [Model Context Protocol](https://modelcontextprotocol.io) server for local developer automation. This revision is the stdio server core: it speaks JSON-RPC 2.0 on stdin/stdout through [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) and exposes one health tool, `ping`.

The process is CGO-free. Logs go to stderr so they never mix with the protocol stream on stdout.

## Status

| Step | Scope | State |
| --- | --- | --- |
| 1 | Project scaffolding, stdio server, `ping` | current |
| 2 | Git workflow tools | not started |
| 3 | Log capture and redaction | not started |
| 4 | Docker and container health | not started |
| 5 | Database schema and migrations | not started |
| 6 | Packaging, release config, final docs | not started |

## Requirements

- Go 1.27 or newer

## Quickstart

```bash
go build -o dev-mcp ./cmd/dev-mcp
```

Build without cgo:

```bash
CGO_ENABLED=0 go build -o dev-mcp ./cmd/dev-mcp
```

On Windows the output file is `dev-mcp.exe`. Running the binary with no arguments waits for an MCP client on stdin. Stop it with Ctrl+C.

## Tools

### `ping`

Read-only health check. It takes no arguments and returns:

| Field | Meaning |
| --- | --- |
| `status` | `ok` when the handler ran |
| `name` | MCP implementation name (`dev-mcp`) |
| `version` | Server version (`0.1.0`) |

## Client configuration

Use the absolute path to the binary you built. The snippets below assume that path.

### Cursor

`.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "dev-mcp": {
      "command": "C:\\path\\to\\dev-mcp.exe"
    }
  }
}
```

### Claude Desktop

`claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "dev-mcp": {
      "command": "C:\\path\\to\\dev-mcp.exe",
      "args": []
    }
  }
}
```

## Architecture

```mermaid
flowchart LR
  client[MCP client]
  main["cmd/dev-mcp"]
  server["internal/server"]
  ping["internal/tools/ping"]

  client -->|"stdio JSON-RPC"| main
  main --> server
  server --> ping
```

```text
dev-mcp/
├── cmd/dev-mcp/           # process entrypoint and signal handling
├── internal/
│   ├── server/            # MCP server, tool registration, stdio transport
│   ├── tools/
│   │   ├── ping/          # health tool
│   │   ├── git/           # placeholder
│   │   ├── logs/          # placeholder
│   │   ├── db/            # placeholder
│   │   └── docker/        # placeholder
│   ├── redaction/         # placeholder
│   └── version/           # name and version advertised to clients
├── .github/workflows/     # go test -race and golangci-lint
├── go.mod
└── README.md
```

`internal/server.New` constructs the SDK server, points its logger at stderr, and registers tools. `internal/server.Run` owns the stdio transport. Tool handlers receive `context.Context` and return before doing work when the context is already cancelled.

## Development

```bash
go test -v -race ./...
golangci-lint run ./...
```

`go test -race` uses the race detector runtime. On Linux and Windows that runtime is built with cgo, so the machine needs a C compiler (GCC on Linux, MinGW-w64 on Windows). The `dev-mcp` binary itself is still built with `CGO_ENABLED=0`. CI provides the compiler.

CI runs both the race tests and golangci-lint v2.15.0 on pull requests and on pushes to `main`.

## Security notes for this revision

- Tool handlers do not invoke a shell. There is no `sh -c` or `bash -c` path.
- `ping` accepts no caller-controlled command text.
- Cancellation is checked in the handler and before `Run` starts the transport.
- Protocol bytes stay on stdout. The default logger writes to stderr.
