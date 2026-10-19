// Command dev-mcp is a stdio Model Context Protocol server for local
// developer automation.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ManiaSacha/dev-mcp/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(server.Options{Logger: logger})
	if err := server.Run(ctx, srv); err != nil && ctx.Err() == nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
