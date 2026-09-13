package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadGates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gates.json")
	writeFile(t, path, `[
		{
			"team_id": "team-1",
			"state_id": "state-review",
			"policy": "advisory",
			"specialist": {"name": "Code Reviewer Flint", "client_id": "id-1", "client_secret": "secret-1"}
		}
	]`)

	gates, err := LoadGates(path)
	if err != nil {
		t.Fatalf("LoadGates: %v", err)
	}
	if len(gates) != 1 {
		t.Fatalf("loaded %d gates, want 1", len(gates))
	}
	gate := gates[0]
	if gate.TeamID != "team-1" || gate.StateID != "state-review" || gate.Policy != PolicyAdvisory {
		t.Errorf("gate = %+v, missing expected fields", gate)
	}
	if gate.Specialist.Name != "Code Reviewer Flint" || gate.Specialist.ClientID != "id-1" || gate.Specialist.ClientSecret != "secret-1" {
		t.Errorf("specialist = %+v, missing expected fields", gate.Specialist)
	}
}

func TestLoadGates_MissingFile(t *testing.T) {
	if _, err := LoadGates(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestLoadGates_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gates.json")
	writeFile(t, path, `not json`)

	if _, err := LoadGates(path); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}
