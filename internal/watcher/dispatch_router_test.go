package watcher

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeDispatchSend writes a shell script standing in for dispatch-send:
// it records its own argv to argsFile, one arg per line, then exits with
// exitCode. Real subprocess, real exec.CommandContext path — no mocking
// of the exec package itself.
func fakeDispatchSend(t *testing.T, exitCode int) (scriptPath, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.txt")
	scriptPath = filepath.Join(dir, "dispatch-send")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + argsFile + "\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), fs.FileMode(0o755)); err != nil {
		t.Fatalf("write fake dispatch-send: %v", err)
	}
	return scriptPath, argsFile
}

func TestDispatchRouter_Route(t *testing.T) {
	scriptPath, argsFile := fakeDispatchSend(t, 0)

	router := DispatchRouter{
		SendPath: scriptPath,
		From:     "pennon-watcher",
		To:       "#pennon",
		Log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	event := Event{
		Action:        "created",
		OAuthClientID: "client-1",
		AgentSession: AgentSession{
			ID:      "session-1",
			IssueID: "issue-1",
			URL:     "https://linear.app/x/agent-sessions/session-1",
		},
	}

	if err := router.Route(event); err != nil {
		t.Fatalf("Route: %v", err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read recorded args: %v", err)
	}
	args := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")

	want := map[string]string{
		"--from":      "pennon-watcher",
		"--to":        "#pennon",
		"--thread-id": "session-1",
	}
	for flag, wantVal := range want {
		got := argAfter(args, flag)
		if got != wantVal {
			t.Errorf("%s = %q, want %q", flag, got, wantVal)
		}
	}

	payloadRaw := argAfter(args, "--payload")
	if payloadRaw == "" {
		t.Fatal("--payload not found in recorded args")
	}
	var payload dispatchPayload
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		t.Fatalf("--payload isn't valid JSON: %v", err)
	}
	if payload.Action != "created" || payload.AgentSessionID != "session-1" || payload.IssueID != "issue-1" || payload.OAuthClientID != "client-1" {
		t.Errorf("payload = %+v, missing expected fields", payload)
	}

	message := args[len(args)-1]
	if !strings.Contains(message, "session-1") || !strings.Contains(message, "created") {
		t.Errorf("trailing message %q doesn't describe the event", message)
	}
}

func TestDispatchRouter_Route_CommandFails(t *testing.T) {
	scriptPath, _ := fakeDispatchSend(t, 1)

	router := DispatchRouter{
		SendPath: scriptPath,
		From:     "pennon-watcher",
		To:       "#pennon",
		Log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	err := router.Route(Event{AgentSession: AgentSession{ID: "session-1"}})
	if err == nil {
		t.Fatal("expected an error when dispatch-send exits non-zero")
	}
}

// argAfter returns the value immediately following flag in args, or "".
func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
