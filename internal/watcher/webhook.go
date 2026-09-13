package watcher

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Deduper decides whether a webhook delivery has already been handled.
// Linear's delivery is at-least-once and unordered (see docs/ARCHITECTURE.md "The watcher"), so
// every event needs an idempotency key — Seen uses the Linear-Delivery
// header, which is unique per delivery attempt including retries of the
// same underlying event.
//
// A no-op implementation (always returns false) is fine for local
// smoke-testing but will double-route on any Linear retry in production.
type Deduper interface {
	// Seen reports whether deliveryID has already been processed, and
	// records it as seen if not — this must be a single atomic
	// check-and-set, not two calls, or two concurrent deliveries of the
	// same retry can both pass.
	Seen(deliveryID string) (bool, error)
}

// Router turns a verified, deduped event into whatever downstream action
// this agent needs — per docs/ARCHITECTURE.md "The watcher," that's meant to be a
// message onto Dispatch (github.com/justinstimatze/mcp-dispatch), not a bespoke queue.
// Deliberately left as an interface here rather than a concrete Dispatch
// client: Dispatch's own filesystem-relay wire format isn't something to
// reimplement from outside without checking with that project first, and
// its IRC gateway and native-bridge are both plausible alternate entry
// points. That choice belongs in a dedicated implementation, not guessed
// at inline in the webhook handler.
type Router interface {
	Route(Event) error
}

// envelope reads just enough of any Linear webhook body — AgentSessionEvent
// and Issue share this shape (see EntityWebhookPayload in schema.graphql)
// — to verify freshness and decide which concrete type to fully decode
// into, before either Router or IssueRouter ever sees it.
type envelope struct {
	Type             string `json:"type"`
	WebhookID        string `json:"webhookId"`
	WebhookTimestamp int64  `json:"webhookTimestamp"`
}

// Handler returns an http.HandlerFunc that verifies and dedupes every
// Linear webhook delivery, then routes it by type: AgentSessionEvent
// goes to route, Issue goes to issueRoute. issueRoute may be nil — a
// deployment with no gates configured (see docs/ARCHITECTURE.md "Deployment config
// lives outside pennon's own repo") simply acks Issue webhooks without
// acting on them, the same optional-fallback pattern LogRouter already
// uses for route. It responds as soon as verification and routing finish
// — callers should keep both Route and RouteIssue fast enough that the
// whole request completes well inside Linear's 5-second ack window (see
// docs/ARCHITECTURE.md "The watcher"); anything that itself needs more than a
// couple seconds belongs in a goroutine kicked off by one of them, not in
// the synchronous path this handler blocks on.
func Handler(secret []byte, dedupe Deduper, route Router, issueRoute IssueRouter, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Error("read webhook body", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := VerifySignature(secret, body, r.Header.Get("Linear-Signature")); err != nil {
			log.Warn("webhook signature rejected", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			log.Error("decode webhook body", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := VerifyFreshness(env.WebhookTimestamp, time.Now()); err != nil {
			log.Warn("webhook rejected", "err", err, "webhook_id", env.WebhookID)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		deliveryID := r.Header.Get("Linear-Delivery")
		seen, err := dedupe.Seen(deliveryID)
		if err != nil {
			log.Error("dedupe check", "err", err, "delivery_id", deliveryID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if seen {
			log.Info("duplicate delivery, acking without re-routing", "delivery_id", deliveryID)
			w.WriteHeader(http.StatusOK)
			return
		}

		switch env.Type {
		case "AgentSessionEvent":
			var event Event
			if err := json.Unmarshal(body, &event); err != nil {
				log.Error("decode AgentSessionEvent body", "err", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := route.Route(event); err != nil {
				log.Error("route event", "err", err, "delivery_id", deliveryID, "agent_session_id", event.AgentSession.ID)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "Issue":
			if issueRoute == nil {
				log.Info("Issue webhook received, no gates configured", "delivery_id", deliveryID)
				break
			}
			var event IssueEvent
			if err := json.Unmarshal(body, &event); err != nil {
				log.Error("decode Issue event body", "err", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := issueRoute.RouteIssue(event); err != nil {
				log.Error("route issue event", "err", err, "delivery_id", deliveryID, "issue_id", event.Data.ID)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		default:
			log.Info("webhook of unhandled type received, acking without routing", "type", env.Type, "delivery_id", deliveryID)
		}

		w.WriteHeader(http.StatusOK)
	}
}
