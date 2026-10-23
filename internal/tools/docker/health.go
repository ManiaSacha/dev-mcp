package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/redaction"
)

// HealthArgs names the container to inspect.
type HealthArgs struct {
	Container string `json:"container" jsonschema:"container name or id"`
}

// HealthLog is one redacted health-check attempt.
type HealthLog struct {
	Start    string `json:"start" jsonschema:"check start time"`
	End      string `json:"end" jsonschema:"check end time"`
	ExitCode int    `json:"exitCode" jsonschema:"check exit code"`
	Output   string `json:"output" jsonschema:"check output after redaction"`
}

// HealthResult is the container state and recent health checks.
type HealthResult struct {
	ID            string      `json:"id" jsonschema:"container id"`
	Name          string      `json:"name" jsonschema:"container name"`
	Status        string      `json:"status" jsonschema:"docker state status"`
	Running       bool        `json:"running" jsonschema:"true when the container is running"`
	ExitCode      int         `json:"exitCode" jsonschema:"last exit code"`
	Error         string      `json:"error,omitempty" jsonschema:"redacted state error, when docker reported one"`
	RestartCount  int         `json:"restartCount" jsonschema:"restart count"`
	Health        string      `json:"health" jsonschema:"healthy, unhealthy, starting, or none when no health check is configured"`
	FailingStreak int         `json:"failingStreak" jsonschema:"consecutive failed health checks"`
	Logs          []HealthLog `json:"logs" jsonschema:"most recent health-check attempts, up to five"`
	LogsTruncated bool        `json:"logsTruncated" jsonschema:"true when older health-check attempts were omitted"`
}

type inspectDocument struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	State        struct {
		Status   string `json:"Status"`
		Running  bool   `json:"Running"`
		ExitCode int    `json:"ExitCode"`
		Error    string `json:"Error"`
		Health   *struct {
			Status        string `json:"Status"`
			FailingStreak int    `json:"FailingStreak"`
			Log           []struct {
				Start    string `json:"Start"`
				End      string `json:"End"`
				ExitCode int    `json:"ExitCode"`
				Output   string `json:"Output"`
			} `json:"Log"`
		} `json:"Health"`
	} `json:"State"`
}

// Health reads one container's state and health check. It does not return
// environment variables, mounts, or other inspect fields.
func (h Handler) Health(ctx context.Context, _ *mcp.CallToolRequest, args HealthArgs) (*mcp.CallToolResult, HealthResult, error) {
	var zero HealthResult
	if err := validateContainer(args.Container); err != nil {
		return nil, zero, toolErr(ToolHealth, err)
	}
	stdout, err := h.dockerOK(ctx, "inspect", "--type", "container", args.Container)
	if err != nil {
		return nil, zero, toolErr(ToolHealth, err)
	}
	result, err := parseHealth(stdout)
	if err != nil {
		return nil, zero, toolErr(ToolHealth, err)
	}
	return nil, result, nil
}

func parseHealth(raw []byte) (HealthResult, error) {
	var docs []inspectDocument
	if err := json.Unmarshal(bytesTrimSpace(raw), &docs); err != nil {
		return HealthResult{}, fmt.Errorf("docker inspect: %w", err)
	}
	if len(docs) != 1 {
		return HealthResult{}, fmt.Errorf("docker inspect returned %d containers", len(docs))
	}
	doc := docs[0]
	errText, _ := redaction.Redact(doc.State.Error)
	result := HealthResult{
		ID:           doc.ID,
		Name:         strings.TrimPrefix(doc.Name, "/"),
		Status:       doc.State.Status,
		Running:      doc.State.Running,
		ExitCode:     doc.State.ExitCode,
		Error:        errText,
		RestartCount: doc.RestartCount,
		Health:       "none",
		Logs:         []HealthLog{},
	}
	if doc.State.Health == nil {
		return result, nil
	}
	result.Health = doc.State.Health.Status
	if result.Health == "" {
		result.Health = "none"
	}
	result.FailingStreak = doc.State.Health.FailingStreak
	logs := doc.State.Health.Log
	if len(logs) > maxHealthLogs {
		result.LogsTruncated = true
		logs = logs[len(logs)-maxHealthLogs:]
	}
	for _, entry := range logs {
		output, _ := redaction.Redact(clipLine(entry.Output))
		result.Logs = append(result.Logs, HealthLog{
			Start:    entry.Start,
			End:      entry.End,
			ExitCode: entry.ExitCode,
			Output:   output,
		})
	}
	return result, nil
}

func bytesTrimSpace(raw []byte) []byte {
	return bytes.TrimSpace(raw)
}
