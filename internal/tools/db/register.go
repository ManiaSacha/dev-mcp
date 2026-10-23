package db

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Handler is the SQLite schema and migration reader.
type Handler struct{}

// Register adds the database tools to s.
func Register(s *mcp.Server) {
	var h Handler
	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolSchema,
		Title:       "Database schema",
		Description: "Read tables, columns, indexes, and foreign keys from a local SQLite file. The file is opened read-only. Rows are not returned, and caller SQL is not accepted.",
		Annotations: readOnlyAnnotations(),
	}, h.Schema)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolMigrations,
		Title:       "Database migrations",
		Description: "List versioned migration files and, when a SQLite file is provided, report which forward versions are not applied. Recognizes golang-migrate and goose trackers. Does not execute SQL.",
		Annotations: readOnlyAnnotations(),
	}, h.Migrations)
}

func readOnlyAnnotations() *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
		OpenWorldHint:  &closed,
	}
}

func toolErr(tool string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", tool, err)
}
