package onboard

import (
	"encoding/json"
	"fmt"
	"os"
)

// PatchSettingsLocal sets two env keys in a worktree's own
// .claude/settings.local.json: ETTLE_ME, so ettle's capture-hook/
// horizon-hook attribute this session's atoms to its own identity
// instead of the shared room default, and PENNON_IDENTITY, pennon's own
// declaration of which identity this worktree is — read back by
// mintForIdentity (cmd/pennon/main.go) to refuse minting any other
// name (see docs/ARCHITECTURE.md "Agent onboarding"). Unlike
// PatchMCPConfig, a missing file is safe to create fresh here — there's
// no pre-existing server entry to invent, just two additive env keys —
// and every other top-level key (enabledPlugins, etc.) is left exactly
// as it was.
func PatchSettingsLocal(path, name string) (PatchResult, error) {
	var result PatchResult

	mode := os.FileMode(0o600)
	cfg := map[string]any{}
	existed := false

	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		existed = true
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode()
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return result, fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
		// Starting from an empty config is fine — see doc comment above.
	default:
		return result, fmt.Errorf("read %s: %w", path, err)
	}

	env, _ := cfg["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
		cfg["env"] = env
	}

	if env["ETTLE_ME"] == name && env["PENNON_IDENTITY"] == name {
		return result, nil
	}
	env["ETTLE_ME"] = name
	env["PENNON_IDENTITY"] = name

	if existed {
		if err := backupFile(path, unixNow); err != nil {
			return result, err
		}
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return result, fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := atomicWriteFile(path, append(out, '\n'), mode); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}

// DeclaredIdentity reads env.PENNON_IDENTITY from a worktree's own
// .claude/settings.local.json — the identity this worktree already
// onboarded as, if any. Returns "" with no error when the file or the
// key doesn't exist yet: a fresh worktree hasn't declared an identity,
// and a worktree onboarded before PENNON_IDENTITY existed (has ETTLE_ME
// but not this key) hasn't been migrated yet either — both are the same
// "nothing declared" case to the caller, not an error.
func DeclaredIdentity(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var cfg struct {
		Env struct {
			PennonIdentity string `json:"PENNON_IDENTITY"`
		} `json:"env"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg.Env.PennonIdentity, nil
}
