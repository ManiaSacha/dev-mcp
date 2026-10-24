package logs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTailAndSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	body := strings.Join([]string{
		"boot ok",
		"login password=hunter2",
		"ready",
		"hello world",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	h := NewHandler()
	_, tail, err := h.Tail(t.Context(), nil, TailArgs{Path: path, Lines: 2})
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if tail.LineCount != 2 || tail.Truncated || tail.LineOrigin != originFile || tail.Redactions != 0 {
		t.Fatalf("tail header = %+v", tail)
	}
	if tail.Lines[0].Number != 3 || tail.Lines[0].Text != "ready" {
		t.Fatalf("first tailed line = %+v", tail.Lines[0])
	}
	if tail.Lines[1].Text != "hello world" {
		t.Fatalf("second tailed line = %+v", tail.Lines[1])
	}

	_, secret, err := h.Tail(t.Context(), nil, TailArgs{Path: path, Lines: 10})
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if secret.Redactions != 1 || secret.Lines[1].Text != "login password=[REDACTED]" {
		t.Fatalf("redacted tail = %+v", secret.Lines)
	}

	_, found, err := h.Search(t.Context(), nil, SearchArgs{Path: path, Query: "hello"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if found.MatchCount != 1 || found.Matches[0].Text != "hello world" || found.Matches[0].Number != 4 {
		t.Fatalf("search = %+v", found)
	}

	_, none, err := h.Search(t.Context(), nil, SearchArgs{Path: path, Query: "h.llo"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if none.MatchCount != 0 {
		t.Fatalf("regex query matched: %+v", none.Matches)
	}

	ignore := true
	_, folded, err := h.Search(t.Context(), nil, SearchArgs{Path: path, Query: "HELLO", IgnoreCase: &ignore})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if folded.MatchCount != 1 {
		t.Fatalf("ignore case = %+v", folded.Matches)
	}
}

func TestTailWindowDropsPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	// The scan window starts inside the first line.
	body := "FIRSTLINE\nsecond\nthird\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	h := Handler{maxScanBytes: 12}
	_, tail, err := h.Tail(t.Context(), nil, TailArgs{Path: path, Lines: 10})
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if !tail.Truncated || tail.LineOrigin != originWindow {
		t.Fatalf("window = %+v", tail)
	}
	for _, line := range tail.Lines {
		if strings.Contains(line.Text, "FIRST") {
			t.Fatalf("partial line was kept: %+v", tail.Lines)
		}
	}
	if len(tail.Lines) == 0 || tail.Lines[len(tail.Lines)-1].Text != "third" {
		t.Fatalf("lines = %+v", tail.Lines)
	}
}

func TestTailRejectsDirectoryAndMissingFile(t *testing.T) {
	h := NewHandler()
	_, _, err := h.Tail(t.Context(), nil, TailArgs{Path: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory error = %v", err)
	}
	_, _, err = h.Tail(t.Context(), nil, TailArgs{Path: filepath.Join(t.TempDir(), "missing.log")})
	if err == nil {
		t.Fatal("missing file succeeded")
	}
	_, _, err = h.Search(t.Context(), nil, SearchArgs{Path: t.TempDir(), Query: "x"})
	if err == nil {
		t.Fatal("search of a directory succeeded")
	}
	_, _, err = h.Search(t.Context(), nil, SearchArgs{Path: filepath.Join(t.TempDir(), "a.log"), Query: ""})
	if err == nil {
		t.Fatal("empty query succeeded")
	}
	_, _, err = h.Tail(t.Context(), nil, TailArgs{Path: filepath.Join(t.TempDir(), "a.log"), Lines: -1})
	if err == nil {
		t.Fatal("negative lines succeeded")
	}
}

func TestRedactTool(t *testing.T) {
	h := NewHandler()
	_, got, err := h.Redact(t.Context(), nil, RedactArgs{Text: "token=abcdef"})
	if err != nil {
		t.Fatalf("Redact() error = %v", err)
	}
	if got.Text != "token=[REDACTED]" || got.Redactions != 1 {
		t.Fatalf("Redact() = %+v", got)
	}
	_, _, err = h.Redact(t.Context(), nil, RedactArgs{Text: strings.Repeat("a", maxRedactBytes+1)})
	if err == nil {
		t.Fatal("oversized text succeeded")
	}
}

func TestCancelledContext(t *testing.T) {
	h := NewHandler()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := h.Tail(ctx, nil, TailArgs{Path: "app.log"})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Tail() error = %v", err)
	}
}

func TestLongLineIsTruncated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	line := strings.Repeat("x", maxLineBytes+50)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	_, tail, err := NewHandler().Tail(t.Context(), nil, TailArgs{Path: path, Lines: 1})
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(tail.Lines) != 1 || !strings.HasSuffix(tail.Lines[0].Text, "…[truncated]") {
		t.Fatalf("line = %+v", tail.Lines)
	}
	if len(tail.Lines[0].Text) > maxLineBytes+len("…[truncated]") {
		t.Fatalf("line length = %d", len(tail.Lines[0].Text))
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
	for _, name := range []string{ToolTail, ToolSearch, ToolRedact} {
		tool := byName[name]
		if tool == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("%s annotations = %+v", name, tool)
		}
	}
	required := schemaRequired(t, byName[ToolSearch].InputSchema)
	if !contains(required, "path") || !contains(required, "query") {
		t.Fatalf("search required = %v", required)
	}
}

func schemaRequired(t *testing.T, schema any) []string {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
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

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestTailMasksMultiLinePrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	body := "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA1234567890abcdef\n-----END RSA PRIVATE KEY-----\nok\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, res, err := NewHandler().Tail(context.Background(), nil, TailArgs{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lines[:3] {
		if l.Text != "[REDACTED]" {
			t.Errorf("line %d leaked: %q", l.Number, l.Text)
		}
	}
	if res.Lines[3].Text != "ok" {
		t.Errorf("last line = %q", res.Lines[3].Text)
	}
}

func TestTailRedactsSecretStraddlingLineLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	// The token starts a few bytes before the 16 KiB cut, so cutting before
	// redaction would leave a short unmasked prefix of it.
	line := strings.Repeat("x", maxLineBytes-12) + " ghp_" + strings.Repeat("A", 36) + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	_, res, err := NewHandler().Tail(context.Background(), nil, TailArgs{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Lines[0].Text
	if strings.Contains(text, "ghp_") || strings.Contains(text, "AAAA") {
		t.Errorf("secret prefix leaked: %q", text[len(text)-30:])
	}
	if !strings.HasSuffix(text, "…[truncated]") && !strings.HasSuffix(text, "[REDACTED]") {
		t.Errorf("unexpected tail: %q", text[len(text)-30:])
	}
}

func TestSearchDoesNotMatchRedactedSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	body := "login password=hunter2 user=ada\nplain line\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler()

	_, res, err := h.Search(context.Background(), nil, SearchArgs{Path: path, Query: "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MatchCount != 0 {
		t.Errorf("query for a secret value matched %d lines", res.MatchCount)
	}

	_, res, err = h.Search(context.Background(), nil, SearchArgs{Path: path, Query: "user=ada"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MatchCount != 1 || strings.Contains(res.Matches[0].Text, "hunter2") {
		t.Errorf("got %+v", res.Matches)
	}
}

func TestTailRespectsAllowedRoots(t *testing.T) {
	allowed := t.TempDir()
	other := t.TempDir()
	for _, dir := range []string{allowed, other} {
		if err := os.WriteFile(filepath.Join(dir, "app.log"), []byte("hello\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DEV_MCP_ALLOWED_ROOTS", allowed)
	h := NewHandler()

	if _, _, err := h.Tail(context.Background(), nil, TailArgs{Path: filepath.Join(allowed, "app.log")}); err != nil {
		t.Errorf("allowed path: %v", err)
	}
	if _, _, err := h.Tail(context.Background(), nil, TailArgs{Path: filepath.Join(other, "app.log")}); err == nil {
		t.Error("path outside the allowed roots was read")
	}
	if _, _, err := h.Search(context.Background(), nil, SearchArgs{Path: filepath.Join(other, "app.log"), Query: "hello"}); err == nil {
		t.Error("search outside the allowed roots succeeded")
	}
}
