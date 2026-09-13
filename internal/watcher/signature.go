// Package watcher receives Linear's AgentSessionEvent webhooks, verifies
// them, dedupes by delivery id, and routes each to the right agent. See
// docs/ARCHITECTURE.md "The watcher" for the design and the confirmed SLA: the
// receiver must respond within 5 seconds, and on a created event the agent
// must emit an activity or update its external URL within 10 seconds or
// Linear marks the session unresponsive.
package watcher

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// maxWebhookAge is how stale a webhook's own webhookTimestamp may be before
// it's rejected as a possible replay. Linear's docs recommend "within a
// minute"; kept as a var, not a const, so a test can tighten it without
// sleeping.
var maxWebhookAge = time.Minute

var (
	// ErrBadSignature means the computed HMAC didn't match the
	// Linear-Signature header — the body, or the header, was tampered
	// with, or the wrong signing secret is configured.
	ErrBadSignature = errors.New("watcher: signature mismatch")
	// ErrStaleWebhook means webhookTimestamp is older than maxWebhookAge.
	// Never returned for a timestamp in the future — clock skew between
	// Linear and this host isn't this function's problem to solve.
	ErrStaleWebhook = errors.New("watcher: webhook timestamp too old")
)

// VerifySignature checks that sig (the raw value of the Linear-Signature
// header, hex-encoded HMAC-SHA256) matches HMAC-SHA256(secret, body).
//
// body must be the exact raw request bytes — Linear's docs are explicit
// that re-serializing a parsed JSON body can produce a different digest
// than the one Linear signed, so this function never accepts anything
// already decoded.
func VerifySignature(secret, body []byte, sig string) error {
	want, err := hex.DecodeString(sig)
	if err != nil {
		return ErrBadSignature
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	got := mac.Sum(nil)
	if !hmac.Equal(got, want) {
		return ErrBadSignature
	}
	return nil
}

// VerifyFreshness checks a webhook's webhookTimestamp (milliseconds since
// the Unix epoch, as sent in the JSON body) against the current time, to
// guard against a captured request being replayed later. Only rejects a
// timestamp older than maxWebhookAge — a timestamp in the future is passed
// through, since penalizing clock skew we don't control buys nothing.
func VerifyFreshness(webhookTimestampMS int64, now time.Time) error {
	sent := time.UnixMilli(webhookTimestampMS)
	if now.Sub(sent) > maxWebhookAge {
		return ErrStaleWebhook
	}
	return nil
}
