package logs

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Register adds the log capture tools to s.
func Register(s *mcp.Server) {
	h := NewHandler()
	closed := false

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolTail,
		Title:       "Tail log",
		Description: "Read the end of a local log file and mask secrets before returning the lines. Large files are scanned from the end, up to 8 MiB. Line numbers are absolute when the whole file fits in that window.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &closed,
		},
	}, h.Tail)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolSearch,
		Title:       "Search log",
		Description: "Find literal text in a local log file and mask secrets in the matching lines. The query is not a regular expression. Large files are scanned from the end, up to 8 MiB.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &closed,
		},
	}, h.Search)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolRedact,
		Title:       "Redact log text",
		Description: "Mask passwords, tokens, private keys, and credential URLs in text. The text is not written to disk.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &closed,
		},
	}, h.Redact)
}
