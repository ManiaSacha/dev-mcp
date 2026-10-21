package git

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStatusAndDiff(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "file.txt", "one\n")
	writeFile(t, repo, "keep.txt", "keep\n")
	writeFile(t, repo, "bin.dat", "\x00\x01\x02")
	gitOK(t, repo, "add", "file.txt", "keep.txt", "bin.dat")
	gitOK(t, repo, "commit", "-m", "init")

	writeFile(t, repo, "file.txt", "one\ntwo\n")
	gitOK(t, repo, "mv", "keep.txt", "renamed.txt")
	writeFile(t, repo, "untracked.txt", "new\n")
	writeFile(t, repo, "bin.dat", "\x00\x01\x02\x03")

	h := NewHandler()
	_, status, err := h.Status(t.Context(), nil, StatusArgs{Path: repo})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Branch != "main" || status.Clean || status.Root != repo {
		t.Fatalf("status header = %+v", status)
	}
	if !hasEntry(status.Entries, "file.txt", "unmodified", "modified") {
		t.Fatalf("missing unstaged file: %+v", status.Entries)
	}
	if !hasRename(status.Entries, "renamed.txt", "keep.txt") {
		t.Fatalf("missing rename: %+v", status.Entries)
	}
	if !hasEntry(status.Entries, "untracked.txt", "untracked", "untracked") {
		t.Fatalf("missing untracked file: %+v", status.Entries)
	}

	_, summary, err := h.DiffSummary(t.Context(), nil, DiffSummaryArgs{Path: repo})
	if err != nil {
		t.Fatalf("DiffSummary() error = %v", err)
	}
	if summary.Staged == nil || summary.Unstaged == nil {
		t.Fatalf("summary = %+v", summary)
	}
	if !hasDiff(*summary.Unstaged, "file.txt", 1, 0, false) {
		t.Fatalf("unstaged = %+v", summary.Unstaged)
	}
	if !hasRenameDiff(*summary.Staged, "renamed.txt", "keep.txt") {
		t.Fatalf("staged rename = %+v", summary.Staged.Files)
	}
	if !hasDiff(*summary.Unstaged, "bin.dat", 0, 0, true) {
		t.Fatalf("binary diff = %+v", summary.Unstaged.Files)
	}

	staged := true
	_, stagedOnly, err := h.DiffSummary(t.Context(), nil, DiffSummaryArgs{Path: repo, Staged: &staged})
	if err != nil {
		t.Fatalf("staged DiffSummary() error = %v", err)
	}
	if stagedOnly.Unstaged != nil || stagedOnly.Staged == nil {
		t.Fatalf("staged filter = %+v", stagedOnly)
	}
}

func TestCreateBranch(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "file.txt", "one\n")
	gitOK(t, repo, "add", "file.txt")
	gitOK(t, repo, "commit", "-m", "init")
	head := strings.TrimSpace(string(gitOK(t, repo, "rev-parse", "HEAD")))

	h := NewHandler()
	_, created, err := h.CreateBranch(t.Context(), nil, CreateBranchArgs{
		Path:       repo,
		Name:       "feature/step-2",
		StartPoint: "HEAD",
	})
	if err != nil {
		t.Fatalf("CreateBranch() error = %v", err)
	}
	if created.Branch != "feature/step-2" || !created.CheckedOut || created.Head != head {
		t.Fatalf("created = %+v, head %s", created, head)
	}
	if current := strings.TrimSpace(string(gitOK(t, repo, "branch", "--show-current"))); current != "feature/step-2" {
		t.Fatalf("current branch = %s", current)
	}

	checkout := false
	_, parked, err := h.CreateBranch(t.Context(), nil, CreateBranchArgs{
		Path:       repo,
		Name:       "parked",
		StartPoint: "HEAD~0",
		Checkout:   &checkout,
	})
	if err != nil {
		t.Fatalf("CreateBranch checkout false error = %v", err)
	}
	if parked.CheckedOut || parked.Head != head {
		t.Fatalf("parked = %+v", parked)
	}
	if current := strings.TrimSpace(string(gitOK(t, repo, "branch", "--show-current"))); current != "feature/step-2" {
		t.Fatalf("current branch after parked = %s", current)
	}

	_, _, err = h.CreateBranch(t.Context(), nil, CreateBranchArgs{Path: repo, Name: "--help"})
	if err == nil {
		t.Fatal("option-like branch name succeeded")
	}
	_, _, err = h.CreateBranch(t.Context(), nil, CreateBranchArgs{Path: repo, Name: "bad..name"})
	if err == nil {
		t.Fatal("invalid branch name succeeded")
	}
	_, _, err = h.CreateBranch(t.Context(), nil, CreateBranchArgs{
		Path:       repo,
		Name:       "from-option",
		StartPoint: "--upload-pack=evil",
	})
	if err == nil {
		t.Fatal("option-like start point succeeded")
	}
	if _, err := os.Stat(filepath.Join(repo, "pwned")); err == nil {
		t.Fatal("start point created an unexpected file")
	}
}

func TestStageAndCommit(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "a.txt", "a\n")
	writeFile(t, repo, "b.txt", "b\n")
	gitOK(t, repo, "add", "a.txt", "b.txt")
	gitOK(t, repo, "commit", "-m", "init")

	writeFile(t, repo, "a.txt", "A\n")
	writeFile(t, repo, "b.txt", "B\n")
	gitOK(t, repo, "add", "b.txt")
	writeFile(t, repo, "my file.txt", "spaced\n")
	writeFile(t, repo, "a;touch pwned.txt", "literal\n")

	h := NewHandler()
	_, committed, err := h.StageAndCommit(t.Context(), nil, StageAndCommitArgs{
		Path:    repo,
		Message: "only selected\n\nbody line",
		Paths:   []string{"a.txt", "my file.txt", "a;touch pwned.txt"},
	})
	if err != nil {
		t.Fatalf("StageAndCommit() error = %v", err)
	}
	if committed.Subject != "only selected" || committed.Branch != "main" || len(committed.Hash) < 40 {
		t.Fatalf("commit = %+v", committed)
	}
	for _, name := range []string{"a.txt", "my file.txt", "a;touch pwned.txt"} {
		if !contains(committed.Files, name) {
			t.Fatalf("files = %v, missing %s", committed.Files, name)
		}
	}
	if contains(committed.Files, "b.txt") {
		t.Fatalf("commit included a previously staged file: %v", committed.Files)
	}
	if _, err := os.Stat(filepath.Join(repo, "pwned")); err == nil {
		t.Fatal("path metacharacters were executed")
	}
	status := string(gitOK(t, repo, "status", "--porcelain=v1"))
	if !strings.Contains(status, "b.txt") {
		t.Fatalf("previously staged file was lost: %s", status)
	}

	_, _, err = h.StageAndCommit(t.Context(), nil, StageAndCommitArgs{
		Path:    repo,
		Message: "nope",
		Paths:   []string{"../outside.txt"},
	})
	if err == nil {
		t.Fatal("path escape succeeded")
	}
	_, _, err = h.StageAndCommit(t.Context(), nil, StageAndCommitArgs{
		Path:    repo,
		Message: " ",
		Paths:   []string{"a.txt"},
	})
	if err == nil {
		t.Fatal("blank message succeeded")
	}
	_, _, err = h.StageAndCommit(t.Context(), nil, StageAndCommitArgs{
		Path:    repo,
		Message: "missing",
		Paths:   []string{"missing.txt"},
	})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing path error = %v", err)
	}

	all := true
	_, allCommit, err := h.StageAndCommit(t.Context(), nil, StageAndCommitArgs{
		Path:    repo,
		Message: "the rest",
		All:     &all,
	})
	if err != nil {
		t.Fatalf("commit all error = %v", err)
	}
	if !contains(allCommit.Files, "b.txt") {
		t.Fatalf("all files = %v", allCommit.Files)
	}
	if clean := string(gitOK(t, repo, "status", "--porcelain=v1")); strings.TrimSpace(clean) != "" {
		t.Fatalf("work tree not clean after all: %s", clean)
	}
}

func TestStageAndCommitDeletion(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "gone.txt", "bye\n")
	gitOK(t, repo, "add", "gone.txt")
	gitOK(t, repo, "commit", "-m", "init")
	if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	h := NewHandler()
	_, committed, err := h.StageAndCommit(t.Context(), nil, StageAndCommitArgs{
		Path:    repo,
		Message: "remove gone",
		Paths:   []string{"gone.txt"},
	})
	if err != nil {
		t.Fatalf("StageAndCommit() error = %v", err)
	}
	if !contains(committed.Files, "gone.txt") {
		t.Fatalf("files = %v", committed.Files)
	}
	if _, err := os.Stat(filepath.Join(repo, "gone.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still present: %v", err)
	}
}

func TestCommitRejectsMissingIdentity(t *testing.T) {
	repo := initRepo(t)
	gitOK(t, repo, "config", "--unset", "user.email")
	gitOK(t, repo, "config", "--unset", "user.name")
	writeFile(t, repo, "file.txt", "one\n")

	h := NewHandler()
	empty := t.TempDir()
	h.env = append(commandEnv(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+filepath.Join(empty, "no-global"),
		"HOME="+empty,
	)
	_, _, err := h.StageAndCommit(t.Context(), nil, StageAndCommitArgs{
		Path:    repo,
		Message: "needs identity",
		Paths:   []string{"file.txt"},
	})
	if err == nil {
		t.Fatal("commit without identity succeeded")
	}
	if _, logErr := exec.Command("git", "-C", repo, "log", "-1").CombinedOutput(); logErr == nil {
		t.Fatal("failed commit still created a revision")
	}
}

func TestCancelledContext(t *testing.T) {
	repo := initRepo(t)
	h := NewHandler()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err := h.Status(ctx, nil, StatusArgs{Path: repo})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Status() error = %v", err)
	}
}

func TestNotARepository(t *testing.T) {
	h := NewHandler()
	_, _, err := h.Status(t.Context(), nil, StatusArgs{Path: t.TempDir()})
	if err == nil {
		t.Fatal("status on a plain directory succeeded")
	}
}

func TestRegisterSchemas(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dev-mcp-test", Version: "0.0.0"}, nil)
	Register(server)
	client := mcp.NewClient(&mcp.Implementation{Name: "dev-mcp-test", Version: "0.0.0"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{ToolStatus, ToolDiffSummary, ToolCreateBranch, ToolStageAndCommit} {
		tool := byName[name]
		if tool == nil {
			t.Fatalf("missing tool %s", name)
		}
		if tool.Annotations == nil || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("%s annotations = %+v", name, tool.Annotations)
		}
		props := schemaProperties(t, tool.InputSchema)
		if _, ok := props["path"]; !ok {
			t.Fatalf("%s input schema missing path: %#v", name, tool.InputSchema)
		}
	}
	if byName[ToolStatus].Annotations == nil || !byName[ToolStatus].Annotations.ReadOnlyHint {
		t.Fatal("git_status is not read-only")
	}
	if byName[ToolStageAndCommit].Annotations == nil || byName[ToolStageAndCommit].Annotations.ReadOnlyHint {
		t.Fatal("git_stage_and_commit is marked read-only")
	}
	required := schemaRequired(t, byName[ToolStageAndCommit].InputSchema)
	if !contains(required, "message") {
		t.Fatalf("commit required = %v", required)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitOK(t, dir, "init", "-b", "main")
	gitOK(t, dir, "config", "user.email", "dev-mcp@example.com")
	gitOK(t, dir, "config", "user.name", "dev-mcp")
	return dir
}

func gitOK(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func hasEntry(entries []StatusEntry, path, index, worktree string) bool {
	for _, entry := range entries {
		if entry.Path == path && entry.Index == index && entry.Worktree == worktree {
			return true
		}
	}
	return false
}

func hasRename(entries []StatusEntry, path, original string) bool {
	for _, entry := range entries {
		if entry.Path == path && entry.OriginalPath == original && entry.Index == "renamed" {
			return true
		}
	}
	return false
}

func hasDiff(side DiffSide, path string, add, del int, binary bool) bool {
	for _, file := range side.Files {
		if file.Path == path && file.Additions == add && file.Deletions == del && file.Binary == binary {
			return true
		}
	}
	return false
}

func hasRenameDiff(side DiffSide, path, original string) bool {
	for _, file := range side.Files {
		if file.Path == path && file.OriginalPath == original {
			return true
		}
	}
	return false
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func schemaProperties(t *testing.T, schema any) map[string]any {
	t.Helper()
	object := schemaObject(t, schema)
	props, ok := object["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type %T in %#v", object["properties"], object)
	}
	return props
}

func schemaRequired(t *testing.T, schema any) []string {
	t.Helper()
	object := schemaObject(t, schema)
	raw, ok := object["required"].([]any)
	if !ok {
		t.Fatalf("required type %T in %#v", object["required"], object)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		name, ok := item.(string)
		if !ok {
			t.Fatalf("required item type %T", item)
		}
		out = append(out, name)
	}
	return out
}

func schemaObject(t *testing.T, schema any) map[string]any {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return object
}
