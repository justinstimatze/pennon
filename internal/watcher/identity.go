package watcher

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadIdentities reads a directory of named identities from a JSON file —
// same shape as a Gate's Specialist, but covering every agent identity a
// deployment names, not just the ones gated on a workflow state.
// Deliberately the same struct as Specialist
// rather than a parallel type: an identity is an identity, whether it's
// reached through a gate or minted directly for a worktree session.
func LoadIdentities(path string) ([]Specialist, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("watcher: read identities %s: %w", path, err)
	}
	var identities []Specialist
	if err := json.Unmarshal(data, &identities); err != nil {
		return nil, fmt.Errorf("watcher: parse identities %s: %w", path, err)
	}
	return identities, nil
}

// FindIdentity returns the identity with the given name, if any.
func FindIdentity(identities []Specialist, name string) (Specialist, bool) {
	for _, id := range identities {
		if id.Name == name {
			return id, true
		}
	}
	return Specialist{}, false
}
