// Package server builds the dev-mcp MCP server and runs it over stdio.
//
// stdout is reserved for JSON-RPC. Server logs go to stderr.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/tools/docker"
	"github.com/ManiaSacha/dev-mcp/internal/tools/git"
	"github.com/ManiaSacha/dev-mcp/internal/tools/logs"
	"github.com/ManiaSacha/dev-mcp/internal/tools/ping"
	"github.com/ManiaSacha/dev-mcp/internal/version"
)

const instructions = `dev-mcp is a local developer automation server.
ping reports the server name, version, and health.
git_status and git_diff_summary read a local git working tree.
git_create_branch creates a local branch.
git_stage_and_commit stages selected changes and commits them.
logs_tail and logs_search read a local log file and mask secrets.
logs_redact masks secrets in text.
docker_info, docker_ps, docker_health, and docker_logs read the local Docker daemon.
These tools never invoke a shell. Write logs to stderr only. stdout carries JSON-RPC.`

// Options configures a server. Zero values use the version package identity
// and a logger that writes to stderr.
type Options struct {
	Name    string
	Version string
	Logger  *slog.Logger
}

// New returns a server with the core tool set registered.
func New(opts Options) *mcp.Server {
	if opts.Name == "" {
		opts.Name = version.Name
	}
	if opts.Version == "" {
		opts.Version = version.Version
	}
	if opts.Logger == nil {
		opts.Logger = stderrLogger()
	}

	s := mcp.NewServer(&mcp.Implementation{
		Name:    opts.Name,
		Version: opts.Version,
	}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       opts.Logger,
	})
	ping.Register(s, ping.Handler{Name: opts.Name, Version: opts.Version})
	git.Register(s)
	logs.Register(s)
	docker.Register(s)
	return s
}

// Run serves s on the process stdin and stdout until the client disconnects
// or ctx is cancelled. The transport is not reused across calls.
func Run(ctx context.Context, s *mcp.Server) error {
	if s == nil {
		return errors.New("server run: nil server")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("server run: %w", err)
	}
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("stdio server: %w", err)
	}
	return nil
}

func stderrLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
