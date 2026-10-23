package docker

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Register adds the Docker health tools to s.
func Register(s *mcp.Server) {
	h := NewHandler()

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolInfo,
		Title:       "Docker info",
		Description: "Report the local Docker client and daemon versions. Uses the server's Docker context and does not change container state.",
		Annotations: readOnlyAnnotations(),
	}, h.Info)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolPS,
		Title:       "List containers",
		Description: "List local containers with state and health. An optional state filter is one of created, restarting, running, paused, exited, or dead. An optional name is a literal substring.",
		Annotations: readOnlyAnnotations(),
	}, h.PS)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolHealth,
		Title:       "Container health",
		Description: "Read one container's status, restart count, and the last five health-check results. Check output is redacted. Environment variables and mounts are not returned.",
		Annotations: readOnlyAnnotations(),
	}, h.Health)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolLogs,
		Title:       "Container logs",
		Description: "Return a redacted tail of one container's logs. The tail length defaults to 100 lines and does not follow the stream.",
		Annotations: readOnlyAnnotations(),
	}, h.Logs)
}

func readOnlyAnnotations() *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
		OpenWorldHint:  &closed,
	}
}
