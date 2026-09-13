package watcher

// Event is the decoded body of an AgentSessionEvent webhook, trimmed to
// the fields the watcher actually routes on. Field shapes are taken
// directly from Linear's published schema
// (AgentSessionEventWebhookPayload / AgentSessionWebhookPayload in
// raw.githubusercontent.com/linear/linear/.../schema.graphql, checked
// 2026-09-10), not guessed from example payloads.
type Event struct {
	// Type is always "AgentSessionEvent" for this webhook category.
	Type string `json:"type"`
	// Action is "created", "prompted", or another lifecycle action —
	// see AgentSession.status for the session's resulting state.
	Action string `json:"action"`
	// AppUserID is the app user (agent identity) this event is for.
	// AgentSessionEvent webhooks only ever fire for one's own app user.
	AppUserID string `json:"appUserId"`
	// OAuthClientID identifies which OAuth Application the app user
	// belongs to — the directory (see docs/ARCHITECTURE.md "The control plane")
	// resolves this to an agent.
	OAuthClientID    string       `json:"oauthClientId"`
	OrganizationID   string       `json:"organizationId"`
	WebhookID        string       `json:"webhookId"`
	WebhookTimestamp int64        `json:"webhookTimestamp"`
	AgentSession     AgentSession `json:"agentSession"`
}

// AgentSession is the lightweight view of a session carried inside a
// webhook payload — not the full queryable AgentSession type.
type AgentSession struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	IssueID   string `json:"issueId"`
	CommentID string `json:"commentId"`
	CreatorID string `json:"creatorId"`
	URL       string `json:"url"`
}
