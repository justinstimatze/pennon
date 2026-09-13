package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPatchMCPConfig_SetsBothTokens(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, ".mcp.json", `{
  "mcpServers": {
    "linear-notifications": {"type": "stdio", "command": "npx", "env": {"LINEAR_API_TOKEN": "old-token"}},
    "linear": {"type": "http", "url": "https://mcp.linear.app/mcp"},
    "someOtherServer": {"type": "stdio", "command": "whatever"}
  }
}`)

	result, err := PatchMCPConfig(path, "new-token")
	if err != nil {
		t.Fatalf("PatchMCPConfig: %v", err)
	}
	if !result.Changed {
		t.Error("Changed = false, want true")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", result.Warnings)
	}

	var cfg map[string]any
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	servers := cfg["mcpServers"].(map[string]any)

	notif := servers["linear-notifications"].(map[string]any)
	if got := notif["env"].(map[string]any)["LINEAR_API_TOKEN"]; got != "new-token" {
		t.Errorf("linear-notifications token = %v, want new-token", got)
	}

	linear := servers["linear"].(map[string]any)
	headers, ok := linear["headers"].(map[string]any)
	if !ok {
		t.Fatal("linear.headers was not created")
	}
	if got := headers["Authorization"]; got != "Bearer new-token" {
		t.Errorf("linear.headers.Authorization = %v, want Bearer new-token", got)
	}
	if linear["url"] != "https://mcp.linear.app/mcp" {
		t.Error("linear.url was disturbed")
	}

	if _, ok := servers["someOtherServer"]; !ok {
		t.Error("unrelated server entry was dropped")
	}

	matches, _ := filepath.Glob(path + ".bak-*")
	if len(matches) != 1 {
		t.Errorf("expected exactly one backup file, got %v", matches)
	}
}

func TestPatchMCPConfig_NoopWhenAlreadyCorrect(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, ".mcp.json", `{
  "mcpServers": {
    "linear-notifications": {"env": {"LINEAR_API_TOKEN": "tok"}},
    "linear": {"headers": {"Authorization": "Bearer tok"}}
  }
}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	result, err := PatchMCPConfig(path, "tok")
	if err != nil {
		t.Fatalf("PatchMCPConfig: %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false when the token already matches")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("file was rewritten despite being a no-op")
	}

	matches, _ := filepath.Glob(path + ".bak-*")
	if len(matches) != 0 {
		t.Errorf("no-op run should not create a backup, got %v", matches)
	}
}

func TestPatchMCPConfig_MissingServerEntriesWarnButDoNotError(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, ".mcp.json", `{"mcpServers": {"unrelated": {"type": "stdio"}}}`)

	result, err := PatchMCPConfig(path, "tok")
	if err != nil {
		t.Fatalf("PatchMCPConfig: %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false — nothing recognizable to patch")
	}
	if len(result.Warnings) != 2 {
		t.Errorf("Warnings = %v, want 2 (linear and linear-notifications both missing)", result.Warnings)
	}
}

func TestPatchMCPConfig_MissingFileSkipsWithoutError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json") // never created

	result, err := PatchMCPConfig(path, "tok")
	if err != nil {
		t.Fatalf("PatchMCPConfig: %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false for a file that doesn't exist")
	}
	if len(result.Warnings) != 1 {
		t.Errorf("Warnings = %v, want exactly one", result.Warnings)
	}
}
