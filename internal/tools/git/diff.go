package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DiffSummaryArgs selects which side of the diff to summarize.
type DiffSummaryArgs struct {
	Path   string `json:"path,omitempty" jsonschema:"path to a git working tree or a directory inside one. Defaults to the server working directory."`
	Staged *bool  `json:"staged,omitempty" jsonschema:"When true, summarize the index only. When false, summarize unstaged worktree changes only. Omit to return both."`
}

// DiffFile is one path in a numstat summary.
type DiffFile struct {
	Path         string `json:"path" jsonschema:"path relative to the work tree. For a rename this is the new path."`
	OriginalPath string `json:"originalPath,omitempty" jsonschema:"previous path when git detected a rename"`
	Additions    int    `json:"additions" jsonschema:"lines added. Zero for a binary file."`
	Deletions    int    `json:"deletions" jsonschema:"lines deleted. Zero for a binary file."`
	Binary       bool   `json:"binary" jsonschema:"true when git reports the file as binary"`
}

// DiffSide is the summary of one diff side.
type DiffSide struct {
	Files     []DiffFile `json:"files" jsonschema:"files with staged or unstaged line changes"`
	Additions int        `json:"additions" jsonschema:"total lines added across text files"`
	Deletions int        `json:"deletions" jsonschema:"total lines deleted across text files"`
	FileCount int        `json:"fileCount" jsonschema:"number of files in this summary"`
}

// DiffSummaryResult holds the requested diff sides.
// Untracked files are omitted; git_status reports those.
type DiffSummaryResult struct {
	Root     string    `json:"root" jsonschema:"absolute path of the working tree"`
	Staged   *DiffSide `json:"staged,omitempty" jsonschema:"index changes, present when staged is omitted or true"`
	Unstaged *DiffSide `json:"unstaged,omitempty" jsonschema:"worktree changes, present when staged is omitted or false"`
}

// DiffSummary returns per-file addition and deletion counts.
func (h Handler) DiffSummary(ctx context.Context, _ *mcp.CallToolRequest, args DiffSummaryArgs) (*mcp.CallToolResult, DiffSummaryResult, error) {
	var zero DiffSummaryResult
	root, err := h.repoRoot(ctx, args.Path)
	if err != nil {
		return nil, zero, toolErr(ToolDiffSummary, err)
	}
	result := DiffSummaryResult{Root: root}
	wantStaged := args.Staged == nil || *args.Staged
	wantUnstaged := args.Staged == nil || !*args.Staged
	if wantStaged {
		side, err := h.numstat(ctx, root, "diff", "--cached", "--numstat", "-z")
		if err != nil {
			return nil, zero, toolErr(ToolDiffSummary, err)
		}
		result.Staged = &side
	}
	if wantUnstaged {
		side, err := h.numstat(ctx, root, "diff", "--numstat", "-z")
		if err != nil {
			return nil, zero, toolErr(ToolDiffSummary, err)
		}
		result.Unstaged = &side
	}
	return nil, result, nil
}

func (h Handler) numstat(ctx context.Context, root string, args ...string) (DiffSide, error) {
	out, err := h.gitStdout(ctx, root, args...)
	if err != nil {
		return DiffSide{}, err
	}
	return parseNumstat(out)
}

func parseNumstat(out []byte) (DiffSide, error) {
	side := DiffSide{Files: []DiffFile{}}
	i := 0
	for i < len(out) {
		rec, next, err := readCString(out, i)
		if err != nil {
			return DiffSide{}, err
		}
		i = next
		added, deleted, path, ok := splitNumstat(rec)
		if !ok {
			return DiffSide{}, fmt.Errorf("bad numstat record %q", rec)
		}
		file := DiffFile{}
		if added == "-" || deleted == "-" {
			file.Binary = true
		} else {
			addN, err := strconv.Atoi(added)
			if err != nil || addN < 0 {
				return DiffSide{}, fmt.Errorf("bad addition count %q", added)
			}
			delN, err := strconv.Atoi(deleted)
			if err != nil || delN < 0 {
				return DiffSide{}, fmt.Errorf("bad deletion count %q", deleted)
			}
			file.Additions = addN
			file.Deletions = delN
			side.Additions += addN
			side.Deletions += delN
		}
		if path == "" {
			old, next, err := readCString(out, i)
			if err != nil {
				return DiffSide{}, fmt.Errorf("rename source: %w", err)
			}
			i = next
			newPath, next, err := readCString(out, i)
			if err != nil {
				return DiffSide{}, fmt.Errorf("rename destination: %w", err)
			}
			i = next
			file.OriginalPath = old
			file.Path = newPath
		} else {
			file.Path = path
		}
		if file.Path == "" {
			return DiffSide{}, fmt.Errorf("numstat record missing path")
		}
		side.Files = append(side.Files, file)
	}
	side.FileCount = len(side.Files)
	return side, nil
}

func splitNumstat(rec string) (added, deleted, path string, ok bool) {
	added, rest, ok := strings.Cut(rec, "\t")
	if !ok {
		return "", "", "", false
	}
	deleted, path, ok = strings.Cut(rest, "\t")
	if !ok {
		return "", "", "", false
	}
	return added, deleted, path, true
}

func readCString(buf []byte, i int) (string, int, error) {
	if i >= len(buf) {
		return "", i, fmt.Errorf("unexpected end of numstat output")
	}
	next := i
	for next < len(buf) && buf[next] != 0 {
		next++
	}
	if next >= len(buf) {
		return "", i, fmt.Errorf("numstat record missing NUL")
	}
	return string(buf[i:next]), next + 1, nil
}
