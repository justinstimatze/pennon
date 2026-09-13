package watcher

import (
	"path/filepath"
	"testing"
)

func TestLoadIdentities(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identities.json")
	writeFile(t, path, `[
		{"name": "mercury@justin", "client_id": "id-1", "client_secret": "secret-1"},
		{"name": "FlintReviewer@justin", "client_id": "id-2", "client_secret": "secret-2"}
	]`)

	identities, err := LoadIdentities(path)
	if err != nil {
		t.Fatalf("LoadIdentities: %v", err)
	}
	if len(identities) != 2 {
		t.Fatalf("loaded %d identities, want 2", len(identities))
	}
}

func TestLoadIdentities_MissingFile(t *testing.T) {
	if _, err := LoadIdentities(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected an error for a missing identities file")
	}
}

func TestFindIdentity(t *testing.T) {
	identities := []Specialist{
		{Name: "mercury@justin", ClientID: "id-1", ClientSecret: "secret-1"},
		{Name: "FlintReviewer@justin", ClientID: "id-2", ClientSecret: "secret-2"},
	}

	t.Run("found", func(t *testing.T) {
		got, ok := FindIdentity(identities, "FlintReviewer@justin")
		if !ok {
			t.Fatal("expected to find FlintReviewer@justin")
		}
		if got.ClientID != "id-2" {
			t.Errorf("ClientID = %q, want id-2", got.ClientID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, ok := FindIdentity(identities, "nobody@justin"); ok {
			t.Error("expected no match for an unknown identity")
		}
	})
}

func TestSpecialist_IsSpecialist(t *testing.T) {
	cases := []struct {
		name string
		kind string
		want bool
	}{
		{"unset", "", false},
		{"agent", "agent", false},
		{"specialist", "specialist", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Specialist{Name: "FlintReviewer@justin", Kind: c.kind}
			if got := s.IsSpecialist(); got != c.want {
				t.Errorf("IsSpecialist() with Kind=%q = %v, want %v", c.kind, got, c.want)
			}
		})
	}
}
