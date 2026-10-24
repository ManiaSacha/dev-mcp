package docker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/tools/toolutil"
)

// InfoArgs has no fields. The tool uses the server's Docker context.
type InfoArgs struct{}

// InfoResult is a small snapshot of the local daemon.
type InfoResult struct {
	Status        string `json:"status" jsonschema:"ok when the daemon responded"`
	ClientVersion string `json:"clientVersion" jsonschema:"docker client version"`
	ServerVersion string `json:"serverVersion" jsonschema:"docker daemon version"`
	APIVersion    string `json:"apiVersion" jsonschema:"docker API version reported by the daemon"`
	OS            string `json:"os" jsonschema:"daemon operating system"`
	Arch          string `json:"arch" jsonschema:"daemon architecture"`
}

type versionDocument struct {
	Client *versionSide `json:"Client"`
	Server *versionSide `json:"Server"`
}

type versionSide struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	OS         string `json:"Os"`
	Arch       string `json:"Arch"`
}

// Info reports client and daemon versions. It does not change Docker state.
func (h Handler) Info(ctx context.Context, _ *mcp.CallToolRequest, _ InfoArgs) (*mcp.CallToolResult, InfoResult, error) {
	var zero InfoResult
	stdout, stderr, code, err := h.run(ctx, "version", "--format", "{{json .}}")
	if err != nil {
		return nil, zero, toolutil.Err(ToolInfo, err)
	}
	result, parseErr := parseInfo(stdout)
	if code != 0 {
		if parseErr == nil && result.ServerVersion == "" {
			return nil, zero, toolutil.Err(ToolInfo, dockerExit([]string{"version"}, stderr, code))
		}
		if parseErr != nil {
			return nil, zero, toolutil.Err(ToolInfo, dockerExit([]string{"version"}, stderr, code))
		}
	}
	if parseErr != nil {
		return nil, zero, toolutil.Err(ToolInfo, parseErr)
	}
	if result.ServerVersion == "" {
		return nil, zero, toolutil.Err(ToolInfo, fmt.Errorf("docker daemon is not reachable"))
	}
	return nil, result, nil
}

func parseInfo(raw []byte) (InfoResult, error) {
	var doc versionDocument
	if err := json.Unmarshal(bytesTrimSpace(raw), &doc); err != nil {
		return InfoResult{}, fmt.Errorf("docker version: %w", err)
	}
	result := InfoResult{Status: "ok"}
	if doc.Client != nil {
		result.ClientVersion = doc.Client.Version
	}
	if doc.Server == nil || doc.Server.Version == "" {
		result.Status = ""
		return result, nil
	}
	result.ServerVersion = doc.Server.Version
	result.APIVersion = doc.Server.APIVersion
	result.OS = doc.Server.OS
	result.Arch = doc.Server.Arch
	return result, nil
}
