package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchSettingsLocal_CreatesEnvBlock(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "settings.local.json", `{"enabledPlugins": {"posthog@claude-plugins-official": true}}`)

	result, err := PatchSettingsLocal(path, "mercury@justin")
	if err != nil {
		t.Fatalf("PatchSettingsLocal: %v", err)
	}
	if !result.Changed {
		t.Error("Changed = false, want true")
	}

	var cfg map[string]any
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	env, ok := cfg["env"].(map[string]any)
	if !ok {
		t.Fatal("env block was not created")
	}
	if env["ETTLE_ME"] != "mercury@justin" {
		t.Errorf("env.ETTLE_ME = %v, want mercury@justin", env["ETTLE_ME"])
	}
	if env["PENNON_IDENTITY"] != "mercury@justin" {
		t.Errorf("env.PENNON_IDENTITY = %v, want mercury@justin", env["PENNON_IDENTITY"])
	}
	plugins, ok := cfg["enabledPlugins"].(map[string]any)
	if !ok || plugins["posthog@claude-plugins-official"] != true {
		t.Error("enabledPlugins was disturbed")
	}
}

func TestPatchSettingsLocal_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json") // never created

	result, err := PatchSettingsLocal(path, "venus@justin")
	if err != nil {
		t.Fatalf("PatchSettingsLocal: %v", err)
	}
	if !result.Changed {
		t.Error("Changed = false, want true")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file was not created: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	env := cfg["env"].(map[string]any)
	if env["ETTLE_ME"] != "venus@justin" {
		t.Errorf("env.ETTLE_ME = %v, want venus@justin", env["ETTLE_ME"])
	}
	if env["PENNON_IDENTITY"] != "venus@justin" {
		t.Errorf("env.PENNON_IDENTITY = %v, want venus@justin", env["PENNON_IDENTITY"])
	}

	matches, _ := filepath.Glob(path + ".bak-*")
	if len(matches) != 0 {
		t.Errorf("creating a fresh file should not write a backup, got %v", matches)
	}
}

func TestPatchSettingsLocal_NoopWhenAlreadyCorrect(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "settings.local.json", `{"env": {"ETTLE_ME": "mars@justin", "PENNON_IDENTITY": "mars@justin"}}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	result, err := PatchSettingsLocal(path, "mars@justin")
	if err != nil {
		t.Fatalf("PatchSettingsLocal: %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false when both keys already match")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("file was rewritten despite being a no-op")
	}
}

func TestPatchSettingsLocal_MigratesMissingPennonIdentity(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "settings.local.json", `{"env": {"ETTLE_ME": "saturn@justin"}}`)

	result, err := PatchSettingsLocal(path, "saturn@justin")
	if err != nil {
		t.Fatalf("PatchSettingsLocal: %v", err)
	}
	if !result.Changed {
		t.Error("Changed = false, want true — a worktree onboarded before PENNON_IDENTITY existed should be migrated")
	}

	var cfg map[string]any
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	env := cfg["env"].(map[string]any)
	if env["ETTLE_ME"] != "saturn@justin" {
		t.Errorf("env.ETTLE_ME = %v, want saturn@justin", env["ETTLE_ME"])
	}
	if env["PENNON_IDENTITY"] != "saturn@justin" {
		t.Errorf("env.PENNON_IDENTITY = %v, want saturn@justin", env["PENNON_IDENTITY"])
	}
}

func TestDeclaredIdentity_MissingFile(t *testing.T) {
	dir := t.TempDir()
	got, err := DeclaredIdentity(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Fatalf("DeclaredIdentity: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string for a missing file", got)
	}
}

func TestDeclaredIdentity_NoEnvBlock(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "settings.local.json", `{"enabledPlugins": {}}`)
	got, err := DeclaredIdentity(path)
	if err != nil {
		t.Fatalf("DeclaredIdentity: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string when there's no env block", got)
	}
}

func TestDeclaredIdentity_NoPennonIdentityKey(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "settings.local.json", `{"env": {"ETTLE_ME": "jupiter@justin"}}`)
	got, err := DeclaredIdentity(path)
	if err != nil {
		t.Fatalf("DeclaredIdentity: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string for a not-yet-migrated worktree", got)
	}
}

func TestDeclaredIdentity_Present(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "settings.local.json", `{"env": {"ETTLE_ME": "mayor@justin", "PENNON_IDENTITY": "mayor@justin"}}`)
	got, err := DeclaredIdentity(path)
	if err != nil {
		t.Fatalf("DeclaredIdentity: %v", err)
	}
	if got != "mayor@justin" {
		t.Errorf("got %q, want mayor@justin", got)
	}
}

func TestDeclaredIdentity_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "settings.local.json", `{not valid json`)
	if _, err := DeclaredIdentity(path); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}
