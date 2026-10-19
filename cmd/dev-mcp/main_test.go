package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/tools/ping"
	"github.com/ManiaSacha/dev-mcp/internal/version"
)

func TestBinaryPingOverStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	bin := filepath.Join(t.TempDir(), "dev-mcp.exe")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	var stderr bytes.Buffer
	// CommandContext cancels the child when ctx ends. Close the session in
	// this function, before that cancel, so a clean stdio shutdown can exit 0.
	// t.Context is already cancelled by the time Cleanup functions run.
	cmd := exec.CommandContext(ctx, bin)
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "dev-mcp-stdio-test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect: %v\nstderr: %s", err, stderr.String())
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: ping.ToolName})
	if err != nil {
		t.Fatalf("CallTool() error = %v\nstderr: %s", err, stderr.String())
	}
	if res.IsError {
		t.Fatalf("ping tool error: %+v\nstderr: %s", res.Content, stderr.String())
	}

	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var got ping.Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	want := ping.Result{Status: "ok", Name: version.Name, Version: version.Version}
	if got != want {
		t.Fatalf("ping = %+v, want %+v\nstderr: %s", got, want, stderr.String())
	}
	// Close while ctx is still active so the child can leave stdio and exit 0.
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v\nstderr: %s", err, stderr.String())
	}
}
