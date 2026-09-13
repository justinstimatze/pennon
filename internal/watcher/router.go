package watcher

import "log/slog"

// LogRouter is the default Router when PENNON_DISPATCH_SEND is unset —
// it logs each event and acks it without routing anywhere, so running
// pennon locally never requires the mcp-dispatch sibling project. See
// DispatchRouter for the real route onto mcp-dispatch's `bin/dispatch-send`
// (docs/ARCHITECTURE.md "Dependency on mcp-dispatch").
//
// LogRouter does NOT satisfy the real 10-second first-activity SLA (see
// docs/ARCHITECTURE.md "The watcher") — logging isn't emitting an AgentActivity
// or updating the session's external URL, so it only belongs in front of
// a live Linear webhook when no routing is configured, never as a
// stand-in for DispatchRouter once one is.
type LogRouter struct {
	Log *slog.Logger
}

// Route implements Router.
func (r LogRouter) Route(event Event) error {
	r.Log.Info("event received, routing not yet wired",
		"action", event.Action,
		"agent_session_id", event.AgentSession.ID,
		"issue_id", event.AgentSession.IssueID,
		"oauth_client_id", event.OAuthClientID,
	)
	return nil
}
