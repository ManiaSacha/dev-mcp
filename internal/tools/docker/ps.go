package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PSArgs filters the container list. Empty fields list every container.
type PSArgs struct {
	State string `json:"state,omitempty" jsonschema:"optional container state: created, restarting, running, paused, exited, or dead"`
	Name  string `json:"name,omitempty" jsonschema:"optional literal substring matched against the container name"`
}

// Container is one row from docker ps.
type Container struct {
	ID     string `json:"id" jsonschema:"container id"`
	Name   string `json:"name" jsonschema:"container name"`
	Image  string `json:"image" jsonschema:"image name"`
	State  string `json:"state" jsonschema:"docker state such as running or exited"`
	Status string `json:"status" jsonschema:"docker status text"`
	Health string `json:"health,omitempty" jsonschema:"healthy, unhealthy, or starting when the status text includes a health check"`
}

// PSResult is the container list.
type PSResult struct {
	Containers []Container `json:"containers" jsonschema:"containers on the local daemon"`
	Count      int         `json:"count" jsonschema:"number of containers returned"`
}

type psRow struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	State  string `json:"State"`
	Status string `json:"Status"`
}

// PS lists containers. State is a fixed filter. Name is applied after docker returns.
func (h Handler) PS(ctx context.Context, _ *mcp.CallToolRequest, args PSArgs) (*mcp.CallToolResult, PSResult, error) {
	var zero PSResult
	if err := validateState(args.State); err != nil {
		return nil, zero, toolErr(ToolPS, err)
	}
	if err := validateNameFilter(args.Name); err != nil {
		return nil, zero, toolErr(ToolPS, err)
	}
	dockerArgs := []string{"ps", "-a", "--no-trunc", "--format", "{{json .}}"}
	if args.State != "" {
		dockerArgs = append(dockerArgs, "--filter", "status="+args.State)
	}
	stdout, err := h.dockerOK(ctx, dockerArgs...)
	if err != nil {
		return nil, zero, toolErr(ToolPS, err)
	}
	containers, err := parsePS(stdout, args.Name)
	if err != nil {
		return nil, zero, toolErr(ToolPS, err)
	}
	return nil, PSResult{Containers: containers, Count: len(containers)}, nil
}

func parsePS(raw []byte, name string) ([]Container, error) {
	containers := []Container{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row psRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		if name != "" && !strings.Contains(row.Names, name) {
			continue
		}
		containers = append(containers, Container{
			ID:     row.ID,
			Name:   row.Names,
			Image:  row.Image,
			State:  row.State,
			Status: row.Status,
			Health: healthFromStatus(row.Status),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return containers, nil
}

func healthFromStatus(status string) string {
	switch {
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	default:
		return ""
	}
}
