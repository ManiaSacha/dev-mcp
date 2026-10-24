package git

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/tools/toolutil"
)

var (
	aheadPattern  = regexp.MustCompile(`\bahead (\d+)\b`)
	behindPattern = regexp.MustCompile(`\bbehind (\d+)\b`)
)

// StatusArgs selects the repository to inspect.
type StatusArgs struct {
	Path string `json:"path,omitempty" jsonschema:"path to a git working tree or a directory inside one. Defaults to the server working directory."`
}

// StatusEntry is one changed, renamed, or untracked path.
type StatusEntry struct {
	Path         string `json:"path" jsonschema:"path relative to the work tree. For a rename this is the new path."`
	OriginalPath string `json:"originalPath,omitempty" jsonschema:"previous path when the entry is a rename or copy"`
	Index        string `json:"index" jsonschema:"staged state: unmodified, modified, added, deleted, renamed, copied, typechange, unmerged, untracked, or ignored"`
	Worktree     string `json:"worktree" jsonschema:"unstaged state, using the same words as index"`
}

// StatusResult is the structured git_status response.
type StatusResult struct {
	Root     string        `json:"root" jsonschema:"absolute path of the working tree"`
	Branch   string        `json:"branch" jsonschema:"current branch name, or HEAD when detached"`
	Detached bool          `json:"detached" jsonschema:"true when HEAD is detached"`
	Upstream string        `json:"upstream,omitempty" jsonschema:"upstream branch, when one is configured"`
	Ahead    int           `json:"ahead" jsonschema:"commits on this branch that are not on the upstream"`
	Behind   int           `json:"behind" jsonschema:"commits on the upstream that are not on this branch"`
	Clean    bool          `json:"clean" jsonschema:"true when there are no staged, unstaged, or untracked entries"`
	Entries  []StatusEntry `json:"entries" jsonschema:"changed, renamed, and untracked paths"`
}

// Status reports the working tree. The context is checked before git runs.
func (h Handler) Status(ctx context.Context, _ *mcp.CallToolRequest, args StatusArgs) (*mcp.CallToolResult, StatusResult, error) {
	var zero StatusResult
	root, err := h.repoRoot(ctx, args.Path)
	if err != nil {
		return nil, zero, toolutil.Err(ToolStatus, err)
	}
	out, err := h.gitStdout(ctx, root, "status", "--porcelain=v1", "-b", "-z")
	if err != nil {
		return nil, zero, toolutil.Err(ToolStatus, err)
	}
	result, err := parseStatus(out)
	if err != nil {
		return nil, zero, toolutil.Err(ToolStatus, err)
	}
	result.Root = root
	return nil, result, nil
}

func parseStatus(out []byte) (StatusResult, error) {
	parts := splitNUL(out)
	if len(parts) == 0 {
		return StatusResult{}, fmt.Errorf("empty git status output")
	}
	result, err := parseHeader(parts[0])
	if err != nil {
		return StatusResult{}, err
	}
	entries := make([]StatusEntry, 0)
	for i := 1; i < len(parts); i++ {
		rec := parts[i]
		if len(rec) < 3 || rec[2] != ' ' {
			return StatusResult{}, fmt.Errorf("bad status record %q", rec)
		}
		index, worktree := rec[0], rec[1]
		entry := StatusEntry{
			Path:     rec[3:],
			Index:    statusWord(index),
			Worktree: statusWord(worktree),
		}
		if index == 'R' || index == 'C' || worktree == 'R' || worktree == 'C' {
			i++
			if i >= len(parts) {
				return StatusResult{}, fmt.Errorf("rename record for %q is missing the original path", entry.Path)
			}
			entry.OriginalPath = parts[i]
		}
		entries = append(entries, entry)
	}
	result.Entries = entries
	result.Clean = len(entries) == 0
	return result, nil
}

func parseHeader(line string) (StatusResult, error) {
	if !strings.HasPrefix(line, "## ") {
		return StatusResult{}, fmt.Errorf("unexpected status header %q", line)
	}
	rest := strings.TrimPrefix(line, "## ")
	var result StatusResult
	switch {
	case rest == "HEAD (no branch)":
		result.Branch = "HEAD"
		result.Detached = true
		return result, nil
	case strings.HasPrefix(rest, "No commits yet on "):
		result.Branch = strings.TrimPrefix(rest, "No commits yet on ")
		result.Entries = []StatusEntry{}
		return result, nil
	}

	name := rest
	if bracket := strings.LastIndex(rest, " ["); bracket >= 0 && strings.HasSuffix(rest, "]") {
		name = rest[:bracket]
		inside := rest[bracket+2 : len(rest)-1]
		result.Ahead = leadingInt(aheadPattern.FindStringSubmatch(inside))
		result.Behind = leadingInt(behindPattern.FindStringSubmatch(inside))
	}
	branch, upstream, ok := strings.Cut(name, "...")
	if branch == "" {
		return StatusResult{}, fmt.Errorf("empty branch in %q", line)
	}
	result.Branch = branch
	if ok {
		result.Upstream = upstream
	}
	result.Entries = []StatusEntry{}
	return result, nil
}

func leadingInt(match []string) int {
	if len(match) < 2 {
		return 0
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return n
}

func statusWord(code byte) string {
	switch code {
	case ' ':
		return "unmodified"
	case 'M':
		return "modified"
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "typechange"
	case 'U':
		return "unmerged"
	case '?':
		return "untracked"
	case '!':
		return "ignored"
	default:
		return "unknown"
	}
}

func splitNUL(out []byte) []string {
	if len(out) == 0 {
		return nil
	}
	parts := bytes.Split(out, []byte{0})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	text := make([]string, len(parts))
	for i, part := range parts {
		text[i] = string(part)
	}
	return text
}
