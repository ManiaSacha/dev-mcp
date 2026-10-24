package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/tools/toolutil"
)

// StageAndCommitArgs selects what to record and the commit message.
type StageAndCommitArgs struct {
	Path    string   `json:"path,omitempty" jsonschema:"path to a git working tree or a directory inside one. Defaults to the server working directory."`
	Message string   `json:"message" jsonschema:"commit message. Leading and trailing whitespace is removed. Must not be empty."`
	Paths   []string `json:"paths,omitempty" jsonschema:"repository-relative paths whose worktree content is committed. Other staged files stay staged and are not included."`
	All     *bool    `json:"all,omitempty" jsonschema:"Stage every change in the work tree, including untracked files and deletions, then commit the index. Cannot be combined with paths."`
}

// StageAndCommitResult describes the commit that was created.
type StageAndCommitResult struct {
	Root     string   `json:"root" jsonschema:"absolute path of the working tree"`
	Branch   string   `json:"branch" jsonschema:"branch the commit landed on, or HEAD when detached"`
	Detached bool     `json:"detached" jsonschema:"true when HEAD is detached"`
	Hash     string   `json:"hash" jsonschema:"full commit hash"`
	Subject  string   `json:"subject" jsonschema:"first line of the commit message"`
	Files    []string `json:"files" jsonschema:"paths recorded in the commit"`
}

// StageAndCommit stages the requested changes and creates one commit.
// The commit is not GPG-signed. Hooks still run. A cancelled context
// stops the git process.
func (h Handler) StageAndCommit(ctx context.Context, _ *mcp.CallToolRequest, args StageAndCommitArgs) (*mcp.CallToolResult, StageAndCommitResult, error) {
	var zero StageAndCommitResult
	message, err := normalizeMessage(args.Message)
	if err != nil {
		return nil, zero, toolutil.Err(ToolStageAndCommit, err)
	}
	paths, all, err := stageSelection(args)
	if err != nil {
		return nil, zero, toolutil.Err(ToolStageAndCommit, err)
	}
	root, err := h.repoRoot(ctx, args.Path)
	if err != nil {
		return nil, zero, toolutil.Err(ToolStageAndCommit, err)
	}
	if err := h.ensureChanges(ctx, root, paths); err != nil {
		return nil, zero, toolutil.Err(ToolStageAndCommit, err)
	}

	if all {
		if err := h.gitOK(ctx, root, nil, "add", "-A"); err != nil {
			return nil, zero, toolutil.Err(ToolStageAndCommit, err)
		}
		if err := h.gitOK(ctx, root, []byte(message), "commit", "--no-gpg-sign", "-F", "-"); err != nil {
			return nil, zero, toolutil.Err(ToolStageAndCommit, err)
		}
	} else {
		if err := h.gitOK(ctx, root, encodePathspecs(paths), "add", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return nil, zero, toolutil.Err(ToolStageAndCommit, err)
		}
		commitArgs := make([]string, 0, 7+len(paths))
		commitArgs = append(commitArgs, "commit", "--no-gpg-sign", "-F", "-", "--only", "--")
		commitArgs = append(commitArgs, paths...)
		if err := h.gitOK(ctx, root, []byte(message), commitArgs...); err != nil {
			return nil, zero, toolutil.Err(ToolStageAndCommit, err)
		}
	}

	result, err := h.commitResult(ctx, root)
	if err != nil {
		return nil, zero, toolutil.Err(ToolStageAndCommit, err)
	}
	return nil, result, nil
}

func (h Handler) ensureChanges(ctx context.Context, root string, paths []string) error {
	args := []string{"status", "--porcelain=v1", "-z", "--"}
	args = append(args, paths...)
	out, err := h.gitStdout(ctx, root, args...)
	if err != nil {
		return err
	}
	if len(splitNUL(out)) > 0 {
		return nil
	}
	for _, rel := range paths {
		_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("path %q does not exist", rel)
		}
		if statErr != nil {
			return statErr
		}
	}
	return errors.New("nothing to commit")
}

func (h Handler) commitResult(ctx context.Context, root string) (StageAndCommitResult, error) {
	hashOut, err := h.gitStdout(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return StageAndCommitResult{}, err
	}
	subjectOut, err := h.gitStdout(ctx, root, "log", "-1", "--format=%s")
	if err != nil {
		return StageAndCommitResult{}, err
	}
	namesOut, err := h.gitStdout(ctx, root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root", "HEAD")
	if err != nil {
		return StageAndCommitResult{}, err
	}
	branchOut, err := h.gitStdout(ctx, root, "branch", "--show-current")
	if err != nil {
		return StageAndCommitResult{}, err
	}
	branch := strings.TrimSpace(string(branchOut))
	detached := branch == ""
	if detached {
		branch = "HEAD"
	}
	files := splitNUL(namesOut)
	if files == nil {
		files = []string{}
	}
	return StageAndCommitResult{
		Root:     root,
		Branch:   branch,
		Detached: detached,
		Hash:     strings.TrimSpace(string(hashOut)),
		Subject:  strings.TrimSpace(string(subjectOut)),
		Files:    files,
	}, nil
}
