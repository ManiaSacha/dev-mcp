package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/tools/toolutil"
)

// CreateBranchArgs names the branch to create.
type CreateBranchArgs struct {
	Path       string `json:"path,omitempty" jsonschema:"path to a git working tree or a directory inside one. Defaults to the server working directory."`
	Name       string `json:"name" jsonschema:"branch name to create, validated with git check-ref-format"`
	StartPoint string `json:"startPoint,omitempty" jsonschema:"commit-ish the branch starts from. Defaults to HEAD."`
	Checkout   *bool  `json:"checkout,omitempty" jsonschema:"Switch the work tree to the new branch. Omit or true to switch. False creates the branch without switching."`
}

// CreateBranchResult describes the branch that was created.
type CreateBranchResult struct {
	Root       string `json:"root" jsonschema:"absolute path of the working tree"`
	Branch     string `json:"branch" jsonschema:"branch that was created"`
	Head       string `json:"head" jsonschema:"full commit hash the branch points at, empty when the branch has no commits yet"`
	CheckedOut bool   `json:"checkedOut" jsonschema:"true when the work tree is on the new branch"`
}

// CreateBranch creates a local branch and, by default, checks it out.
func (h Handler) CreateBranch(ctx context.Context, _ *mcp.CallToolRequest, args CreateBranchArgs) (*mcp.CallToolResult, CreateBranchResult, error) {
	var zero CreateBranchResult
	if err := validateBranchName(args.Name); err != nil {
		return nil, zero, toolutil.Err(ToolCreateBranch, err)
	}
	if err := validateStartPoint(args.StartPoint); err != nil {
		return nil, zero, toolutil.Err(ToolCreateBranch, err)
	}
	root, err := h.repoRoot(ctx, args.Path)
	if err != nil {
		return nil, zero, toolutil.Err(ToolCreateBranch, err)
	}
	if err := h.gitOK(ctx, root, nil, "check-ref-format", "--branch", args.Name); err != nil {
		return nil, zero, toolutil.Err(ToolCreateBranch, err)
	}
	if args.StartPoint != "" {
		rev := args.StartPoint + "^{commit}"
		if err := h.gitOK(ctx, root, nil, "rev-parse", "--verify", "--end-of-options", rev); err != nil {
			return nil, zero, toolutil.Err(ToolCreateBranch, err)
		}
	}

	checkout := args.Checkout == nil || *args.Checkout
	if checkout {
		// The branch name is already rejected when it starts with "-".
		// "--" belongs before the start point: `git switch -c <name> --`
		// otherwise treats the new name as a commit to switch to.
		gitArgs := []string{"switch", "-c", args.Name}
		if args.StartPoint != "" {
			gitArgs = append(gitArgs, "--", args.StartPoint)
		}
		if err := h.gitOK(ctx, root, nil, gitArgs...); err != nil {
			return nil, zero, toolutil.Err(ToolCreateBranch, err)
		}
	} else {
		gitArgs := []string{"branch", "--", args.Name}
		if args.StartPoint != "" {
			gitArgs = append(gitArgs, args.StartPoint)
		}
		if err := h.gitOK(ctx, root, nil, gitArgs...); err != nil {
			return nil, zero, toolutil.Err(ToolCreateBranch, err)
		}
	}

	result, err := h.branchResult(ctx, root, args.Name, checkout)
	if err != nil {
		return nil, zero, toolutil.Err(ToolCreateBranch, err)
	}
	return nil, result, nil
}

func (h Handler) branchResult(ctx context.Context, root, name string, checkout bool) (CreateBranchResult, error) {
	result := CreateBranchResult{Root: root, Branch: name, CheckedOut: checkout}
	stdout, _, code, err := h.run(ctx, root, nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return CreateBranchResult{}, err
	}
	if code == 0 {
		result.Head = strings.TrimSpace(string(stdout))
	}
	if !checkout {
		return result, nil
	}
	current, err := h.gitStdout(ctx, root, "branch", "--show-current")
	if err != nil {
		return CreateBranchResult{}, err
	}
	if strings.TrimSpace(string(current)) != name {
		return CreateBranchResult{}, fmt.Errorf("branch %q was created but HEAD is %q", name, strings.TrimSpace(string(current)))
	}
	return result, nil
}
