// Package onboard collapses the manual steps a worktree needs after
// `pennon mint` — wiring the minted token into its own .mcp.json and
// setting ettle's per-session identity in .claude/settings.local.json —
// into single, idempotent patch functions. See docs/ARCHITECTURE.md's "Agent
// onboarding" for the manual sequence this replaces.
package onboard

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to path via create-temp-in-same-dir →
// write → chmod → close → rename, so an interruption (or a live Claude
// Code session reading the file mid-write) never sees a half-written or
// truncated file — ported from be-my-geminis' cmd/bmg/atomic.go, which
// exists for exactly the same class of file (Claude Code config a live
// session reads on every operation).
//
// This does NOT serialize against a concurrent writer — if something
// else rewrites path between the caller's read and this write, that
// update is silently lost under the caller's version. The backup file
// callers write before calling this is the recovery path for that case.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomic write: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".onboard-atomic-*.tmp")
	if err != nil {
		return fmt.Errorf("atomic write: create temp in %s: %w", dir, err)
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("atomic write: write %s: %w", name, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("atomic write: chmod %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("atomic write: close %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return fmt.Errorf("atomic write: rename to %s: %w", path, err)
	}
	return nil
}

// backupFile copies path to a `.bak-<unixtime>` sibling before it gets
// mutated — the recovery path for the race atomicWriteFile doesn't
// serialize against. A no-op (not an error) when path doesn't exist yet,
// since there's nothing to preserve.
func backupFile(path string, now func() int64) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("backup %s: %w", path, err)
	}
	backupPath := fmt.Sprintf("%s.bak-%d", path, now())
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return fmt.Errorf("write backup %s: %w", backupPath, err)
	}
	return nil
}
