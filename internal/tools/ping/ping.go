// Package ping implements the dev-mcp health tool.
package ping

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/version"
)

// ToolName is the MCP tool name clients call.
const ToolName = "ping"

// Args is the ping input. The tool takes no parameters.
type Args struct{}

// Result is the structured ping response.
type Result struct {
	Status  string `json:"status" jsonschema:"server health status"`
	Version string `json:"version" jsonschema:"dev-mcp server version"`
	Name    string `json:"name" jsonschema:"MCP implementation name"`
}

// Handler reports server liveness. Name and Version are returned as-is
// when set; otherwise Call falls back to the package defaults.
type Handler struct {
	Name    string
	Version string
}

// Call handles a ping invocation. A cancelled context is returned to the
// caller so the SDK can surface it as a tool error.
func (h Handler) Call(ctx context.Context, _ *mcp.CallToolRequest, _ Args) (*mcp.CallToolResult, Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, Result{}, fmt.Errorf("ping cancelled: %w", err)
	}

	name := h.Name
	if name == "" {
		name = version.Name
	}
	ver := h.Version
	if ver == "" {
		ver = version.Version
	}

	return nil, Result{
		Status:  "ok",
		Version: ver,
		Name:    name,
	}, nil
}

// Register adds the ping tool to s. Empty handler identity fields are
// filled from the version package before the handler is stored.
func Register(s *mcp.Server, h Handler) {
	if h.Name == "" {
		h.Name = version.Name
	}
	if h.Version == "" {
		h.Version = version.Version
	}

	openWorld := false
	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolName,
		Title:       "Ping",
		Description: "Health check. Returns the dev-mcp server name, version, and status.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &openWorld,
		},
	}, h.Call)
}
