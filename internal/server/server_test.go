package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/tools/db"
	"github.com/ManiaSacha/dev-mcp/internal/tools/docker"
	"github.com/ManiaSacha/dev-mcp/internal/tools/git"
	"github.com/ManiaSacha/dev-mcp/internal/tools/logs"
	"github.com/ManiaSacha/dev-mcp/internal/tools/ping"
	"github.com/ManiaSacha/dev-mcp/internal/version"
)

func TestNewRegistersPing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts Options
		want ping.Result
	}{
		{
			name: "defaults",
			want: ping.Result{
				Status:  "ok",
				Name:    version.Name,
				Version: version.Version,
			},
		},
		{
			name: "overrides",
			opts: Options{Name: "custom", Version: "9.9.9"},
			want: ping.Result{
				Status:  "ok",
				Name:    "custom",
				Version: "9.9.9",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			session := connect(t, New(quiet(tt.opts)))
			tool := mustPingTool(t, session)

			if tool.Description == "" {
				t.Fatal("ping description is empty")
			}
			if schemaType(t, tool.InputSchema) != "object" {
				t.Fatalf("input schema type = %v, want object", schemaType(t, tool.InputSchema))
			}
			assertOutputProperties(t, tool.OutputSchema)

			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: ping.ToolName})
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			got := decodePing(t, res)
			if got != tt.want {
				t.Fatalf("ping = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRunRejectsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := Run(ctx, New(quiet(Options{})))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunRejectsNilServer(t *testing.T) {
	t.Parallel()

	err := Run(t.Context(), nil)
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
}

func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "dev-mcp-test", Version: "0.0.0"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() {
		if err := serverSession.Close(); err != nil {
			t.Errorf("close server session: %v", err)
		}
	})

	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close client session: %v", err)
		}
	})
	return session
}

func TestNewRegistersGitTools(t *testing.T) {
	t.Parallel()

	session := connect(t, New(quiet(Options{})))
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	want := []string{
		db.ToolMigrations,
		db.ToolSchema,
		docker.ToolHealth,
		docker.ToolInfo,
		docker.ToolLogs,
		docker.ToolPS,
		git.ToolCreateBranch,
		git.ToolDiffSummary,
		git.ToolStageAndCommit,
		git.ToolStatus,
		logs.ToolRedact,
		logs.ToolSearch,
		logs.ToolTail,
		ping.ToolName,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tools = %v, want %v", got, want)
	}
}

func mustPingTool(t *testing.T, session *mcp.ClientSession) *mcp.Tool {
	t.Helper()

	tool := mustTool(t, session, ping.ToolName)
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Fatal("ping is not marked read-only")
	}
	return tool
}

func mustTool(t *testing.T, session *mcp.ClientSession, name string) *mcp.Tool {
	t.Helper()

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q is not registered", name)
	return nil
}

func decodePing(t *testing.T, res *mcp.CallToolResult) ping.Result {
	t.Helper()
	if res.IsError {
		t.Fatalf("ping tool error: %+v", res.Content)
	}

	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var got ping.Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	return got
}

func schemaType(t *testing.T, schema any) string {
	t.Helper()
	object, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("schema type %T, want map[string]any", schema)
	}
	kind, _ := object["type"].(string)
	return kind
}

func assertOutputProperties(t *testing.T, schema any) {
	t.Helper()
	object, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("output schema type %T, want map[string]any", schema)
	}
	props, ok := object["properties"].(map[string]any)
	if !ok {
		t.Fatalf("output properties type %T", object["properties"])
	}
	for _, key := range []string{"status", "version", "name"} {
		if _, ok := props[key]; !ok {
			t.Errorf("output schema missing %q", key)
		}
	}
}

func quiet(opts Options) Options {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return opts
}
