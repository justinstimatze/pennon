package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// dispatchSendTimeout bounds the dispatch-send subprocess call so a hung
// or slow relay can't itself blow the watcher's 5-second webhook ack SLA
// (see docs/ARCHITECTURE.md "The watcher").
const dispatchSendTimeout = 3 * time.Second

// DispatchRouter routes an Event onto mcp-dispatch by shelling out to
// bin/dispatch-send — the CLI mcp-dispatch built (commit 7a9accf,
// v0.11.6) specifically so a non-MCP process like this watcher can post
// one message without hand-rolling their filesystem wire format. See
// docs/ARCHITECTURE.md "Dependency on mcp-dispatch."
type DispatchRouter struct {
	// SendPath is the absolute path to dispatch-send.
	SendPath string
	// From is this watcher's own dispatch sender id.
	From string
	// To is who receives every routed event — an agent id, a nick, or a
	// '#channel'. There's no per-agent directory yet (see docs/ARCHITECTURE.md
	// "The control plane" — not built), so every event goes to this one
	// fixed destination until one exists.
	To  string
	Log *slog.Logger
}

// dispatchPayload is the structured form of an Event carried in
// dispatch-send's --payload, so the receiving agent doesn't have to
// parse it back out of the human-readable message text.
type dispatchPayload struct {
	Action         string `json:"action"`
	AgentSessionID string `json:"agent_session_id"`
	IssueID        string `json:"issue_id"`
	OAuthClientID  string `json:"oauth_client_id"`
	URL            string `json:"url"`
}

// Route implements Router.
func (r DispatchRouter) Route(event Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), dispatchSendTimeout)
	defer cancel()

	payload, err := json.Marshal(dispatchPayload{
		Action:         event.Action,
		AgentSessionID: event.AgentSession.ID,
		IssueID:        event.AgentSession.IssueID,
		OAuthClientID:  event.OAuthClientID,
		URL:            event.AgentSession.URL,
	})
	if err != nil {
		return fmt.Errorf("watcher: marshal dispatch payload: %w", err)
	}

	message := fmt.Sprintf("Linear AgentSessionEvent: action=%s session=%s issue=%s",
		event.Action, event.AgentSession.ID, event.AgentSession.IssueID)

	// exec.CommandContext takes argv directly, no shell — webhook-derived
	// fields (issue ids, session ids) land as literal argv elements, not
	// through any shell interpolation, so there's nothing here for a
	// crafted Linear payload to inject into.
	cmd := exec.CommandContext(ctx, r.SendPath,
		"--from", r.From,
		"--to", r.To,
		"--thread-id", event.AgentSession.ID,
		"--payload", string(payload),
		message,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("watcher: dispatch-send failed: %w: %s", err, out)
	}

	r.Log.Info("routed to dispatch", "to", r.To, "agent_session_id", event.AgentSession.ID, "action", event.Action)
	return nil
}
