package watcher

// IssueEvent is the decoded body of an Issue webhook, trimmed to the
// fields the gate mechanism routes on. Field shapes are taken from
// Linear's published schema (EntityWebhookPayload / IssueWebhookPayload
// in schema.graphql, checked 2026-09-10), not guessed from example
// payloads.
type IssueEvent struct {
	// Type is always "Issue" for this webhook category.
	Type string `json:"type"`
	// Action is "create", "update", or "remove".
	Action string `json:"action"`
	// OrganizationID identifies the Linear workspace this event belongs to.
	OrganizationID string `json:"organizationId"`
	// UpdatedFrom carries the previous value of every field that changed
	// on an "update" action. It's the only way to tell a genuine state
	// *transition* (previous stateId differs from the current one) apart
	// from an update that merely happened while the issue already sat in
	// the gate state — a title edit shouldn't re-trigger delegation.
	UpdatedFrom      map[string]any `json:"updatedFrom"`
	Data             IssueData      `json:"data"`
	WebhookID        string         `json:"webhookId"`
	WebhookTimestamp int64          `json:"webhookTimestamp"`
}

// IssueData is the lightweight view of an issue carried inside a webhook
// payload — not the full queryable Issue type.
type IssueData struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	TeamID     string `json:"teamId"`
	StateID    string `json:"stateId"`
	URL        string `json:"url"`
}

// TransitionedInto reports whether this update moved the issue's
// workflow state to stateID, as opposed to the issue merely being
// updated while already sitting in that state. Only "update" actions can
// ever be a transition — "create" and "remove" never carry a prior
// stateId to compare against.
func (e IssueEvent) TransitionedInto(stateID string) bool {
	if e.Action != "update" || e.Data.StateID != stateID {
		return false
	}
	prev, ok := e.UpdatedFrom["stateId"]
	if !ok {
		return false
	}
	prevID, ok := prev.(string)
	return ok && prevID != stateID
}
