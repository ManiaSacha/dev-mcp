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

	"github.com/ManiaSacha/dev-mcp/internal/tools/git"
	"github.com/ManiaSacha/dev-mcp/internal/tools/logs"
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

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	initRepo := exec.CommandContext(ctx, "git", "init", "-b", "main")
	initRepo.Dir = repo
	if out, err := initRepo.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	statusRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      git.ToolStatus,
		Arguments: git.StatusArgs{Path: repo},
	})
	if err != nil {
		t.Fatalf("git_status error = %v\nstderr: %s", err, stderr.String())
	}
	if statusRes.IsError {
		t.Fatalf("git_status tool error: %+v\nstderr: %s", statusRes.Content, stderr.String())
	}
	statusData, err := json.Marshal(statusRes.StructuredContent)
	if err != nil {
		t.Fatalf("marshal git_status: %v", err)
	}
	var status git.StatusResult
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatalf("unmarshal git_status: %v", err)
	}
	if status.Branch != "main" || !status.Clean {
		t.Fatalf("git_status = %+v\nstderr: %s", status, stderr.String())
	}

	redactRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      logs.ToolRedact,
		Arguments: logs.RedactArgs{Text: "password=hunter2"},
	})
	if err != nil {
		t.Fatalf("logs_redact error = %v\nstderr: %s", err, stderr.String())
	}
	if redactRes.IsError {
		t.Fatalf("logs_redact tool error: %+v\nstderr: %s", redactRes.Content, stderr.String())
	}
	redactData, err := json.Marshal(redactRes.StructuredContent)
	if err != nil {
		t.Fatalf("marshal logs_redact: %v", err)
	}
	var redacted logs.RedactResult
	if err := json.Unmarshal(redactData, &redacted); err != nil {
		t.Fatalf("unmarshal logs_redact: %v", err)
	}
	if redacted.Text != "password=[REDACTED]" || redacted.Redactions != 1 {
		t.Fatalf("logs_redact = %+v\nstderr: %s", redacted, stderr.String())
	}
	// Close while ctx is still active so the child can leave stdio and exit 0.
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v\nstderr: %s", err, stderr.String())
	}
}

func TestVersionFlag(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	bin := filepath.Join(t.TempDir(), "dev-mcp")
	build := exec.CommandContext(ctx, "go", "build",
		"-ldflags", "-X github.com/ManiaSacha/dev-mcp/internal/version.Version=9.9.9",
		"-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	cmd := exec.CommandContext(ctx, bin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--version: %v\n%s", err, out)
	}
	if string(out) != "dev-mcp 9.9.9\n" {
		t.Fatalf("--version = %q", out)
	}
}
