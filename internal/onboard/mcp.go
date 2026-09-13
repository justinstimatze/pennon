package onboard

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// PatchResult reports what a patch function actually did: whether it
// wrote anything, and any keys it couldn't touch because the config
// didn't have the shape it expected.
type PatchResult struct {
	Changed  bool
	Warnings []string
}

// PatchMCPConfig wires a freshly minted token into a worktree's own
// .mcp.json: mcpServers["linear-notifications"].env.LINEAR_API_TOKEN
// (the existing PAT-shaped server) and
// mcpServers["linear"].headers.Authorization (Linear's hosted server,
// which accepts a plain bearer header with no separate OAuth handshake
// — found live 2026-09-10, see docs/ARCHITECTURE.md "Agent onboarding").
//
// Reads the whole file into map[string]any rather than a typed struct
// so every server entry and field this doesn't know about survives
// untouched — the same idiom used across this codebase's sibling tools
// for this exact problem (be-my-geminis' mergeClaudeJSON, plancheck's
// setupMCP). A missing file, or a missing server entry inside it, is
// reported as a warning and skipped rather than invented from nothing —
// this function patches what's there, it doesn't scaffold new config.
func PatchMCPConfig(path, token string) (PatchResult, error) {
	var result PatchResult

	mode := os.FileMode(0o600)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s does not exist yet — skipped", path))
			return result, nil
		}
		return result, fmt.Errorf("read %s: %w", path, err)
	}
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode()
	}

	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return result, fmt.Errorf("parse %s: %w", path, err)
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s has no mcpServers — skipped", path))
		return result, nil
	}

	dirty := false

	if entry, ok := servers["linear-notifications"].(map[string]any); ok {
		env, _ := entry["env"].(map[string]any)
		if env == nil {
			env = map[string]any{}
			entry["env"] = env
		}
		if env["LINEAR_API_TOKEN"] != token {
			env["LINEAR_API_TOKEN"] = token
			dirty = true
		}
	} else {
		result.Warnings = append(result.Warnings, `mcpServers["linear-notifications"] not found — skipped`)
	}

	if entry, ok := servers["linear"].(map[string]any); ok {
		headers, _ := entry["headers"].(map[string]any)
		if headers == nil {
			headers = map[string]any{}
			entry["headers"] = headers
		}
		want := "Bearer " + token
		if headers["Authorization"] != want {
			headers["Authorization"] = want
			dirty = true
		}
	} else {
		result.Warnings = append(result.Warnings, `mcpServers["linear"] not found — skipped`)
	}

	if !dirty {
		return result, nil
	}

	if err := backupFile(path, unixNow); err != nil {
		return result, err
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

func unixNow() int64 { return time.Now().Unix() }
