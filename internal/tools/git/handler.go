// Package git implements local git workflow tools for dev-mcp.
//
// Commands are executed as argument vectors with exec.CommandContext.
// Nothing is passed through a shell. Repository paths are the child
// process working directory, and caller-controlled names are rejected
// when they could be interpreted as options.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ManiaSacha/dev-mcp/internal/pathguard"
)

const (
	// ToolStatus is the read-only working tree status tool.
	ToolStatus = "git_status"
	// ToolCreateBranch creates a local branch.
	ToolCreateBranch = "git_create_branch"
	// ToolStageAndCommit stages selected changes and creates a commit.
	ToolStageAndCommit = "git_stage_and_commit"
	// ToolDiffSummary summarizes staged and unstaged line changes.
	ToolDiffSummary = "git_diff_summary"
)

// Handler runs git against a local working tree.
// A zero Handler is not usable; call NewHandler.
type Handler struct {
	git     string
	lookErr error
	// env replaces the child environment when non-nil. Tests use it to
	// hide the developer gitconfig. Production leaves it nil.
	env []string
}

// NewHandler resolves the git executable on PATH.
// The handler still registers when git is missing; calls then return lookErr.
func NewHandler() Handler {
	path, err := exec.LookPath("git")
	if err != nil {
		return Handler{lookErr: fmt.Errorf("git not found on PATH: %w", err)}
	}
	return Handler{git: path}
}

func (h Handler) repoRoot(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir := path
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("working directory: %w", err)
		}
		dir = wd
	}
	if strings.ContainsRune(dir, 0) || strings.ContainsAny(dir, "\r\n") {
		return "", errors.New("path contains a newline")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("path: %w", err)
	}
	if err := pathguard.Check(abs); err != nil {
		return "", fmt.Errorf("path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %s is not a directory", abs)
	}

	out, err := h.gitStdout(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("git did not return a work tree")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	// git works on the whole work tree, which can sit above the path the
	// caller passed. Check the root too, so an allowed folder inside a larger
	// repository cannot expose or commit the rest of that repository.
	if err := pathguard.Check(root); err != nil {
		return "", fmt.Errorf("repository root %s: %w", root, err)
	}
	return root, nil
}

func (h Handler) gitStdout(ctx context.Context, dir string, args ...string) ([]byte, error) {
	stdout, stderr, code, err := h.run(ctx, dir, nil, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, h.exitError(args, stderr, code)
	}
	return stdout, nil
}

func (h Handler) gitOK(ctx context.Context, dir string, stdin []byte, args ...string) error {
	_, stderr, code, err := h.run(ctx, dir, stdin, args...)
	if err != nil {
		return err
	}
	if code != 0 {
		return h.exitError(args, stderr, code)
	}
	return nil
}

func (h Handler) exitError(args []string, stderr []byte, code int) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", code)
	}
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	name := "git"
	if len(args) > 0 {
		name = args[0]
	}
	return fmt.Errorf("git %s: %s", name, msg)
}

func (h Handler) run(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	if h.lookErr != nil {
		return nil, nil, -1, h.lookErr
	}
	if h.git == "" {
		return nil, nil, -1, errors.New("git not found on PATH")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, -1, err
	}

	cmd := exec.CommandContext(ctx, h.git, args...)
	cmd.Dir = dir
	cmd.Env = h.childEnv()
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdout.Bytes(), stderr.Bytes(), -1, ctxErr
	}
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
	}
	return stdout.Bytes(), stderr.Bytes(), -1, err
}

func (h Handler) childEnv() []string {
	if h.env != nil {
		return h.env
	}
	return commandEnv()
}

func commandEnv() []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GIT_TERMINAL_PROMPT", "GIT_LITERAL_PATHSPECS", "GIT_PAGER":
			continue
		default:
			env = append(env, entry)
		}
	}
	return append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_PAGER=cat",
	)
}
