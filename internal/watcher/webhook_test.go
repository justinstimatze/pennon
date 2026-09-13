package watcher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeDeduper struct {
	seen map[string]bool
}

func (f *fakeDeduper) Seen(deliveryID string) (bool, error) {
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	was := f.seen[deliveryID]
	f.seen[deliveryID] = true
	return was, nil
}

type fakeRouter struct {
	routed []Event
	err    error
}

func (f *fakeRouter) Route(e Event) error {
	f.routed = append(f.routed, e)
	return f.err
}

var errRouteFailed = errors.New("route failed")

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validBody(webhookTimestampMS int64) []byte {
	return []byte(fmt.Sprintf(
		`{"type":"AgentSessionEvent","action":"created","appUserId":"app-1","oauthClientId":"client-1","organizationId":"org-1","webhookId":"wh-1","webhookTimestamp":%d,"agentSession":{"id":"session-1","status":"pending","issueId":"issue-1"}}`,
		webhookTimestampMS,
	))
}

func newRequest(secret, body []byte, deliveryID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/linear", bytes.NewReader(body))
	req.Header.Set("Linear-Signature", sign(secret, body))
	req.Header.Set("Linear-Delivery", deliveryID)
	return req
}

func TestHandler(t *testing.T) {
	secret := []byte("whsec_test")

	t.Run("valid event routes and acks", func(t *testing.T) {
		router := &fakeRouter{}
		dedupe := &fakeDeduper{}
		h := Handler(secret, dedupe, router, nil, discardLogger())

		body := validBody(time.Now().UnixMilli())
		rec := httptest.NewRecorder()
		h(rec, newRequest(secret, body, "delivery-1"))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if len(router.routed) != 1 {
			t.Fatalf("routed %d events, want 1", len(router.routed))
		}
		if router.routed[0].AgentSession.ID != "session-1" {
			t.Errorf("routed event AgentSession.ID = %q, want session-1", router.routed[0].AgentSession.ID)
		}
	})

	t.Run("bad signature is rejected before routing", func(t *testing.T) {
		router := &fakeRouter{}
		dedupe := &fakeDeduper{}
		h := Handler(secret, dedupe, router, nil, discardLogger())

		body := validBody(time.Now().UnixMilli())
		req := newRequest([]byte("wrong-secret"), body, "delivery-2")
		rec := httptest.NewRecorder()
		h(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if len(router.routed) != 0 {
			t.Fatalf("routed %d events, want 0 (bad signature must never route)", len(router.routed))
		}
	})

	t.Run("stale webhook is rejected before routing", func(t *testing.T) {
		router := &fakeRouter{}
		dedupe := &fakeDeduper{}
		h := Handler(secret, dedupe, router, nil, discardLogger())

		old := time.Now().Add(-2 * time.Hour).UnixMilli()
		req := newRequest(secret, validBody(old), "delivery-3")
		rec := httptest.NewRecorder()
		h(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if len(router.routed) != 0 {
			t.Fatalf("routed %d events, want 0 (stale webhook must never route)", len(router.routed))
		}
	})

	t.Run("duplicate delivery acks without re-routing", func(t *testing.T) {
		router := &fakeRouter{}
		dedupe := &fakeDeduper{}
		h := Handler(secret, dedupe, router, nil, discardLogger())

		body := validBody(time.Now().UnixMilli())
		h(httptest.NewRecorder(), newRequest(secret, body, "delivery-4"))
		rec := httptest.NewRecorder()
		h(rec, newRequest(secret, body, "delivery-4"))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (a duplicate should still ack)", rec.Code)
		}
		if len(router.routed) != 1 {
			t.Fatalf("routed %d events, want 1 (duplicate must not re-route)", len(router.routed))
		}
	})

	t.Run("router error surfaces as 500 so Linear retries", func(t *testing.T) {
		router := &fakeRouter{err: errRouteFailed}
		dedupe := &fakeDeduper{}
		h := Handler(secret, dedupe, router, nil, discardLogger())

		body := validBody(time.Now().UnixMilli())
		rec := httptest.NewRecorder()
		h(rec, newRequest(secret, body, "delivery-5"))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
	})

	t.Run("Issue webhook routes to issueRoute, not route", func(t *testing.T) {
		router := &fakeRouter{}
		issueRouter := &fakeIssueRouter{}
		dedupe := &fakeDeduper{}
		h := Handler(secret, dedupe, router, issueRouter, discardLogger())

		body := validIssueBody(time.Now().UnixMilli())
		rec := httptest.NewRecorder()
		h(rec, newRequest(secret, body, "delivery-6"))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if len(router.routed) != 0 {
			t.Errorf("Issue webhook routed %d times to the AgentSessionEvent router, want 0", len(router.routed))
		}
		if len(issueRouter.routed) != 1 {
			t.Fatalf("routed %d issue events, want 1", len(issueRouter.routed))
		}
		if issueRouter.routed[0].Data.ID != "issue-1" {
			t.Errorf("routed issue event Data.ID = %q, want issue-1", issueRouter.routed[0].Data.ID)
		}
	})

	t.Run("Issue webhook with no issueRoute configured just acks", func(t *testing.T) {
		router := &fakeRouter{}
		dedupe := &fakeDeduper{}
		h := Handler(secret, dedupe, router, nil, discardLogger())

		body := validIssueBody(time.Now().UnixMilli())
		rec := httptest.NewRecorder()
		h(rec, newRequest(secret, body, "delivery-7"))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (unconfigured Issue routing must still ack)", rec.Code)
		}
	})
}

type fakeIssueRouter struct {
	routed []IssueEvent
	err    error
}

func (f *fakeIssueRouter) RouteIssue(e IssueEvent) error {
	f.routed = append(f.routed, e)
	return f.err
}

func validIssueBody(webhookTimestampMS int64) []byte {
	return []byte(fmt.Sprintf(
		`{"type":"Issue","action":"update","organizationId":"org-1","webhookId":"wh-2","webhookTimestamp":%d,"updatedFrom":{"stateId":"state-todo"},"data":{"id":"issue-1","identifier":"CUR-1","teamId":"team-1","stateId":"state-review","url":"https://linear.app/x/issue/CUR-1"}}`,
		webhookTimestampMS,
	))
}
