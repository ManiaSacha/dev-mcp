// Package docker reports local Docker daemon and container health.
//
// Every command is an argument vector. Nothing is passed through a shell.
// Container names that start with "-" are rejected before Docker sees them.
// Health-check output and container logs are masked before they are returned.
package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ManiaSacha/dev-mcp/internal/redaction"
)

const (
	// ToolInfo reports whether the local Docker daemon is reachable.
	ToolInfo = "docker_info"
	// ToolPS lists containers and their health.
	ToolPS = "docker_ps"
	// ToolHealth inspects one container's health check.
	ToolHealth = "docker_health"
	// ToolLogs returns a redacted tail of one container's logs.
	ToolLogs = "docker_logs"

	defaultLogLines = 100
	maxLogLines     = 1000
	maxHealthLogs   = 5
	maxLineBytes    = 16 << 10
	maxOutputBytes  = 1 << 20
)

// Handler runs the docker CLI. A zero Handler is not usable; call NewHandler.
type Handler struct {
	docker  string
	lookErr error
	// env replaces the child environment when non-nil. Tests use it to
	// point a fake docker binary at fixture output.
	env []string
}

// NewHandler resolves the docker executable on PATH.
// The handler still registers when docker is missing; calls then return lookErr.
func NewHandler() Handler {
	path, err := exec.LookPath("docker")
	if err != nil {
		return Handler{lookErr: fmt.Errorf("docker not found on PATH: %w", err)}
	}
	return Handler{docker: path}
}

func (h Handler) run(ctx context.Context, args ...string) (stdout []byte, stderr []byte, code int, err error) {
	if err = ctx.Err(); err != nil {
		return nil, nil, -1, err
	}
	if h.lookErr != nil {
		return nil, nil, -1, h.lookErr
	}
	if h.docker == "" {
		return nil, nil, -1, errors.New("docker not found on PATH")
	}

	cmd := exec.CommandContext(ctx, h.docker, args...)
	cmd.Env = h.childEnv()
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return outBuf.Bytes(), errBuf.Bytes(), -1, ctxErr
	}
	if err == nil {
		return outBuf.Bytes(), errBuf.Bytes(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return outBuf.Bytes(), errBuf.Bytes(), exitErr.ExitCode(), nil
	}
	return outBuf.Bytes(), errBuf.Bytes(), -1, err
}

func (h Handler) dockerOK(ctx context.Context, args ...string) ([]byte, error) {
	stdout, stderr, code, err := h.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, dockerExit(args, stderr, code)
	}
	return stdout, nil
}

func dockerExit(args []string, stderr []byte, code int) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", code)
	}
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	msg, _ = redaction.Redact(msg)
	name := "docker"
	if len(args) > 0 {
		name = args[0]
	}
	return fmt.Errorf("docker %s: %s", name, msg)
}

func (h Handler) childEnv() []string {
	if h.env != nil {
		return h.env
	}
	base := os.Environ()
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if key == "DOCKER_CLI_HINTS" {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "DOCKER_CLI_HINTS=false")
}

func toolErr(tool string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", tool, err)
}

func bounded(n, fallback, max int, name string) (int, error) {
	if n == 0 {
		return fallback, nil
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	if n > max {
		return 0, fmt.Errorf("%s must be at most %d", name, max)
	}
	return n, nil
}

func clipLine(text string) string {
	if len(text) <= maxLineBytes {
		return text
	}
	return text[:maxLineBytes] + "…[truncated]"
}
