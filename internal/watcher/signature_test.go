package watcher

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	secret := []byte("whsec_test")
	body := []byte(`{"type":"AgentSessionEvent","action":"created"}`)

	t.Run("valid signature", func(t *testing.T) {
		if err := VerifySignature(secret, body, sign(secret, body)); err != nil {
			t.Fatalf("VerifySignature() = %v, want nil", err)
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		if err := VerifySignature([]byte("wrong"), body, sign(secret, body)); err != ErrBadSignature {
			t.Fatalf("VerifySignature() = %v, want ErrBadSignature", err)
		}
	})

	t.Run("tampered body", func(t *testing.T) {
		sig := sign(secret, body)
		tampered := []byte(`{"type":"AgentSessionEvent","action":"deleted"}`)
		if err := VerifySignature(secret, tampered, sig); err != ErrBadSignature {
			t.Fatalf("VerifySignature() = %v, want ErrBadSignature", err)
		}
	})

	t.Run("malformed hex signature", func(t *testing.T) {
		if err := VerifySignature(secret, body, "not-hex!!"); err != ErrBadSignature {
			t.Fatalf("VerifySignature() = %v, want ErrBadSignature", err)
		}
	})

	t.Run("empty signature", func(t *testing.T) {
		if err := VerifySignature(secret, body, ""); err != ErrBadSignature {
			t.Fatalf("VerifySignature() = %v, want ErrBadSignature", err)
		}
	})
}

func TestVerifyFreshness(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	t.Run("just sent", func(t *testing.T) {
		sent := now.Add(-time.Second).UnixMilli()
		if err := VerifyFreshness(sent, now); err != nil {
			t.Fatalf("VerifyFreshness() = %v, want nil", err)
		}
	})

	t.Run("exactly at the boundary is not stale", func(t *testing.T) {
		sent := now.Add(-maxWebhookAge).UnixMilli()
		if err := VerifyFreshness(sent, now); err != nil {
			t.Fatalf("VerifyFreshness() = %v, want nil", err)
		}
	})

	t.Run("past the boundary is stale", func(t *testing.T) {
		sent := now.Add(-maxWebhookAge - time.Second).UnixMilli()
		if err := VerifyFreshness(sent, now); err != ErrStaleWebhook {
			t.Fatalf("VerifyFreshness() = %v, want ErrStaleWebhook", err)
		}
	})

	t.Run("future timestamp is not rejected", func(t *testing.T) {
		sent := now.Add(time.Hour).UnixMilli()
		if err := VerifyFreshness(sent, now); err != nil {
			t.Fatalf("VerifyFreshness() = %v, want nil (clock skew isn't this function's problem)", err)
		}
	})
}
