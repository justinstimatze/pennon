package onboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Due reports whether an identity's onboard-hook refresh is due — and, if
// so, records "now" before returning, exactly like ettle's own
// dueForCapture. Ported from cmd/ettle/capture.go, same reasoning:
// recording the attempt rather than the success means a transient
// failure doesn't retry-storm the endpoint, it just waits out the same
// window and tries again next session start.
//
// Keyed by identity name alone — deliberately the OPPOSITE of ettle's
// own per-session keying. ettle's dueForCapture comment describes a real
// bug: keying on room alone silently dropped captures from parallel
// sessions publishing genuinely different atoms. That doesn't apply
// here — a pennon refresh is idempotent per identity, re-running it
// twice does nothing a single run wouldn't, and this fleet runs one
// session per identity at a time. SessionStart also refires on resume,
// /clear, and /compact, not just a fresh launch, which is exactly the
// repeated firing this debounce exists to collapse. Don't "fix" this
// into per-session keying later — that reintroduces the duplicate-mint,
// duplicate-.bak-file waste the debounce was built to prevent.
//
// Identity names (mercury@justin, FlintReviewer@justin) are already
// filesystem-safe as written — no sanitizing step, unlike ettle's own
// transport.SanitizeID, which exists for arbitrary room/transport specs
// pennon's identity names don't need.
func Due(name string, window time.Duration) (bool, error) {
	path, err := duePath(name)
	if err != nil {
		return false, err
	}
	if data, err := os.ReadFile(path); err == nil {
		if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			if time.Since(ts) < window {
				return false, nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("record onboard-hook attempt for %s: %w", name, err)
	}
	if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return false, fmt.Errorf("record onboard-hook attempt for %s: %w", name, err)
	}
	return true, nil
}

func duePath(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "pennon", "onboard", name+".hookrun"), nil
}
