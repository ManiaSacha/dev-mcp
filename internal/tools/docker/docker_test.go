package docker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseAndTools(t *testing.T) {
	bin := writeFake(t)
	argsFile := filepath.Join(t.TempDir(), "args")

	version := `{"Client":{"Version":"27.0.0","ApiVersion":"1.46"},"Server":{"Version":"27.0.0","ApiVersion":"1.46","Os":"linux","Arch":"amd64"}}`
	h := fakeHandler(bin, argsFile, "version", version)
	_, info, err := h.Info(t.Context(), nil, InfoArgs{})
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Status != "ok" || info.ServerVersion != "27.0.0" || info.OS != "linux" || info.Arch != "amd64" {
		t.Fatalf("Info() = %+v", info)
	}

	ps := "{\"ID\":\"abc123\",\"Names\":\"web\",\"Image\":\"app:latest\",\"State\":\"running\",\"Status\":\"Up 2 hours (healthy)\"}\n"
	h = fakeHandler(bin, argsFile, "ps", ps)
	_, listed, err := h.PS(t.Context(), nil, PSArgs{State: "running", Name: "web"})
	if err != nil {
		t.Fatalf("PS() error = %v", err)
	}
	if listed.Count != 1 || listed.Containers[0].Health != "healthy" || listed.Containers[0].Name != "web" {
		t.Fatalf("PS() = %+v", listed)
	}
	gotArgs := readArgs(t, argsFile)
	if !strings.Contains(gotArgs, "--filter\nstatus=running") {
		t.Fatalf("ps args = %q", gotArgs)
	}
	if strings.Contains(gotArgs, "web") {
		t.Fatalf("name filter was passed to docker: %q", gotArgs)
	}

	inspect := `[{"Id":"abc123","Name":"/web","RestartCount":2,"State":{"Status":"running","Running":true,"ExitCode":0,"Error":"","Health":{"Status":"healthy","FailingStreak":0,"Log":[{"Start":"t0","End":"t1","ExitCode":0,"Output":"ok"},{"Start":"t2","End":"t3","ExitCode":1,"Output":"password=examplevalue"}]}}}]`
	h = fakeHandler(bin, argsFile, "inspect", inspect)
	_, health, err := h.Health(t.Context(), nil, HealthArgs{Container: "web"})
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.Name != "web" || health.Health != "healthy" || health.RestartCount != 2 {
		t.Fatalf("Health() = %+v", health)
	}
	if len(health.Logs) != 2 || health.Logs[1].Output != "password=[REDACTED]" {
		t.Fatalf("health logs = %+v", health.Logs)
	}
	if strings.Contains(readArgs(t, argsFile), "sh\n-c") || strings.Contains(readArgs(t, argsFile), "--help") {
		t.Fatalf("inspect args = %q", readArgs(t, argsFile))
	}

	h = fakeHandler(bin, argsFile, "logs", "")
	_, logs, err := h.Logs(t.Context(), nil, LogsArgs{Container: "web", Lines: 20})
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if logs.Redactions != 1 || len(logs.Lines) != 2 {
		t.Fatalf("Logs() = %+v", logs)
	}
	if logs.Lines[0].Stream != "stdout" || logs.Lines[0].Text != "app started" {
		t.Fatalf("stdout = %+v", logs.Lines[0])
	}
	if logs.Lines[1].Stream != "stderr" || logs.Lines[1].Text != "password=[REDACTED]" {
		t.Fatalf("stderr = %+v", logs.Lines[1])
	}
	if !strings.Contains(readArgs(t, argsFile), "--tail\n20\nweb") {
		t.Fatalf("logs args = %q", readArgs(t, argsFile))
	}
}

func TestHealthWithoutCheck(t *testing.T) {
	raw := `[{"Id":"abc","Name":"/job","RestartCount":0,"State":{"Status":"exited","Running":false,"ExitCode":1,"Health":null}}]`
	got, err := parseHealth([]byte(raw))
	if err != nil {
		t.Fatalf("parseHealth() error = %v", err)
	}
	if got.Health != "none" || got.Running || len(got.Logs) != 0 {
		t.Fatalf("parseHealth() = %+v", got)
	}
}

func TestPSNameFilterAndHealthWords(t *testing.T) {
	raw := "{\"ID\":\"1\",\"Names\":\"api\",\"Image\":\"api\",\"State\":\"running\",\"Status\":\"Up (unhealthy)\"}\n{\"ID\":\"2\",\"Names\":\"worker\",\"Image\":\"worker\",\"State\":\"running\",\"Status\":\"Up (health: starting)\"}\n"
	got, err := parsePS([]byte(raw), "work")
	if err != nil {
		t.Fatalf("parsePS() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "worker" || got[0].Health != "starting" {
		t.Fatalf("parsePS() = %+v", got)
	}
}

func TestRejectsUnsafeContainers(t *testing.T) {
	bin := writeFake(t)
	argsFile := filepath.Join(t.TempDir(), "args")
	h := fakeHandler(bin, argsFile, "logs", "")
	for _, container := range []string{"--help", "name;rm", "../etc", "", "has space"} {
		_, _, err := h.Logs(t.Context(), nil, LogsArgs{Container: container})
		if err == nil {
			t.Fatalf("container %q was accepted", container)
		}
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Fatal("docker ran for a rejected container")
	}
	_, _, err := h.PS(t.Context(), nil, PSArgs{State: "running;rm"})
	if err == nil {
		t.Fatal("unsafe state was accepted")
	}
	_, _, err = h.Logs(t.Context(), nil, LogsArgs{Container: "web", Lines: -1})
	if err == nil {
		t.Fatal("negative lines was accepted")
	}
}

func TestDaemonDown(t *testing.T) {
	bin := writeFake(t)
	argsFile := filepath.Join(t.TempDir(), "args")
	h := fakeHandler(bin, argsFile, "fail", `{"Client":{"Version":"27.0.0","ApiVersion":"1.46"}}`)
	_, _, err := h.Info(t.Context(), nil, InfoArgs{})
	if err == nil || !strings.Contains(err.Error(), "daemon") {
		t.Fatalf("Info() error = %v", err)
	}
}

func TestCancelledContext(t *testing.T) {
	h := NewHandler()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := h.PS(ctx, nil, PSArgs{})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("PS() error = %v", err)
	}
}

func writeFake(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake docker is a shell script, which Windows cannot run")
	}
	path := filepath.Join(t.TempDir(), "docker")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$DOCKER_FAKE_ARGS"
case "$DOCKER_FAKE_MODE" in
  fail)
    printf '%s' "$DOCKER_FAKE_OUT"
    echo daemon down >&2
    exit 1
    ;;
  logs)
    printf '%s\n' "app started"
    printf '%s\n' "password=examplevalue" >&2
    ;;
  *)
    printf '%s' "$DOCKER_FAKE_OUT"
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	return path
}

func fakeHandler(bin, argsFile, mode, out string) Handler {
	return Handler{
		docker: bin,
		env: []string{
			"DOCKER_FAKE_ARGS=" + argsFile,
			"DOCKER_FAKE_MODE=" + mode,
			"DOCKER_FAKE_OUT=" + out,
		},
	}
}

func readArgs(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return string(data)
}
