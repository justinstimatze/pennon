package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMintForIdentity_RefusesDeclaredMismatch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte(`{"env": {"PENNON_IDENTITY": "alpha"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	identitiesPath := filepath.Join(dir, "identities.json")
	fixture := `[
		{"name": "alpha", "client_id": "id-alpha", "client_secret": "secret-alpha"},
		{"name": "beta", "client_id": "id-beta", "client_secret": "secret-beta"}
	]`
	if err := os.WriteFile(identitiesPath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PENNON_IDENTITIES", identitiesPath)

	_, err := mintForIdentity(context.Background(), "beta")
	if err == nil {
		t.Fatal("expected an error minting a name other than the one this worktree declared")
	}
	if !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Errorf("error %q should name both the declared identity and the requested one", err.Error())
	}
}

func TestMintForIdentity_AllowsDeclaredIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte(`{"env": {"PENNON_IDENTITY": "alpha"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	identitiesPath := filepath.Join(dir, "identities.json")
	fixture := `[{"name": "alpha", "client_id": "id-alpha", "client_secret": "secret-alpha"}]`
	if err := os.WriteFile(identitiesPath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PENNON_IDENTITIES", identitiesPath)

	// alpha's own mint proceeds past the declared-identity guard and fails
	// on the network call instead (there's no real Linear credential
	// behind this fixture) — that's the correct boundary for this test to
	// stop at, not a live MintToken call.
	_, err := mintForIdentity(context.Background(), "alpha")
	if err == nil {
		t.Fatal("expected an error from the (fake) network call, not a clean success")
	}
	if strings.Contains(err.Error(), "is onboarded as") {
		t.Errorf("declared-identity guard should not fire for the identity a worktree actually declared, got: %v", err)
	}
}
