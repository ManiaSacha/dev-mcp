package docker

import (
	"context"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/redaction"
	"github.com/ManiaSacha/dev-mcp/internal/tools/toolutil"
)

// LogsArgs selects a container and how many log lines to return.
type LogsArgs struct {
	Container  string `json:"container" jsonschema:"container name or id"`
	Lines      int    `json:"lines,omitempty" jsonschema:"lines to return from the end. Defaults to 100. Maximum 1000."`
	Timestamps *bool  `json:"timestamps,omitempty" jsonschema:"prefix each line with its timestamp when true"`
}

// OutputLine is one redacted container log line.
type OutputLine struct {
	Stream string `json:"stream" jsonschema:"stdout or stderr"`
	Text   string `json:"text" jsonschema:"line text after redaction"`
}

// LogsResult is a redacted log tail.
type LogsResult struct {
	Container  string       `json:"container" jsonschema:"container that was read"`
	Lines      []OutputLine `json:"lines" jsonschema:"stdout lines first, then stderr lines"`
	LineCount  int          `json:"lineCount" jsonschema:"number of lines returned"`
	Redactions int          `json:"redactions" jsonschema:"number of secrets replaced"`
	Truncated  bool         `json:"truncated" jsonschema:"true when the returned text hit the size limit"`
}

// Logs tails one container's logs and masks secrets. It does not follow the stream.
func (h Handler) Logs(ctx context.Context, _ *mcp.CallToolRequest, args LogsArgs) (*mcp.CallToolResult, LogsResult, error) {
	var zero LogsResult
	if err := validateContainer(args.Container); err != nil {
		return nil, zero, toolutil.Err(ToolLogs, err)
	}
	n, err := toolutil.Bounded(args.Lines, defaultLogLines, maxLogLines, "lines")
	if err != nil {
		return nil, zero, toolutil.Err(ToolLogs, err)
	}
	dockerArgs := []string{"logs", "--tail", strconv.Itoa(n)}
	if args.Timestamps != nil && *args.Timestamps {
		dockerArgs = append(dockerArgs, "--timestamps")
	}
	dockerArgs = append(dockerArgs, args.Container)
	stdout, stderr, code, err := h.run(ctx, dockerArgs...)
	if err != nil {
		return nil, zero, toolutil.Err(ToolLogs, err)
	}
	if code != 0 {
		return nil, zero, toolutil.Err(ToolLogs, dockerExit(dockerArgs, stderr, code))
	}
	lines, redactions, truncated := collectLogs(stdout, stderr)
	return nil, LogsResult{
		Container:  args.Container,
		Lines:      lines,
		LineCount:  len(lines),
		Redactions: redactions,
		Truncated:  truncated,
	}, nil
}

func collectLogs(stdout, stderr []byte) ([]OutputLine, int, bool) {
	lines := make([]OutputLine, 0)
	redactions := 0
	used := 0
	truncated := false
	appendStream := func(stream string, raw []byte) {
		if truncated {
			return
		}
		text := strings.TrimSuffix(string(raw), "\n")
		if text == "" && len(raw) == 0 {
			return
		}
		parts := strings.Split(text, "\n")
		for i, part := range parts {
			parts[i] = strings.TrimSuffix(part, "\r")
		}
		maskedParts, _ := redaction.RedactLines(parts)
		for i, masked := range maskedParts {
			changed := masked != parts[i]
			masked = redaction.Clip(masked, maxLineBytes)
			if used+len(masked) > maxOutputBytes {
				truncated = true
				return
			}
			used += len(masked)
			if changed {
				redactions++
			}
			lines = append(lines, OutputLine{Stream: stream, Text: masked})
		}
	}
	appendStream("stdout", stdout)
	appendStream("stderr", stderr)
	return lines, redactions, truncated
}
