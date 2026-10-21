package git

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Register adds the git workflow tools to s.
func Register(s *mcp.Server) {
	h := NewHandler()
	closed := false
	additive := false
	destructive := true

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolStatus,
		Title:       "Git status",
		Description: "Read a local git working tree. Returns the branch, upstream ahead/behind counts, and one entry per staged, unstaged, renamed, or untracked path.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &closed,
		},
	}, h.Status)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolDiffSummary,
		Title:       "Git diff summary",
		Description: "Summarize added and deleted lines per file for the index, the work tree, or both. Renames include the original path. Untracked files are omitted; use git_status for those.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &closed,
		},
	}, h.DiffSummary)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolCreateBranch,
		Title:       "Create git branch",
		Description: "Create a local branch after git check-ref-format accepts the name. Switches to the new branch unless checkout is false. An optional startPoint selects the starting commit.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: &additive,
			IdempotentHint:  false,
			OpenWorldHint:   &closed,
		},
	}, h.CreateBranch)

	mcp.AddTool(s, &mcp.Tool{
		Name:        ToolStageAndCommit,
		Title:       "Stage and commit",
		Description: "Commit local changes without a shell. paths commits the worktree content of those files and leaves other staged files untouched. all stages every change, including untracked files, and commits the index. The commit is not signed. Git hooks still run.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: &destructive,
			IdempotentHint:  false,
			OpenWorldHint:   &closed,
		},
	}, h.StageAndCommit)
}
