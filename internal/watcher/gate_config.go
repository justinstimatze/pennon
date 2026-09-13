package watcher

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadGates reads a GateSet from a JSON file — the deployment-specific
// config described in docs/ARCHITECTURE.md "Deployment config lives outside
// pennon's own repo." Which team, which state, and which specialist are
// a per-deployment concern; pennon only knows how to read the shape (see
// config/gate.example.json for that shape with placeholder values).
func LoadGates(path string) (GateSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("watcher: read gate config %s: %w", path, err)
	}
	var gates GateSet
	if err := json.Unmarshal(data, &gates); err != nil {
		return nil, fmt.Errorf("watcher: parse gate config %s: %w", path, err)
	}
	return gates, nil
}
