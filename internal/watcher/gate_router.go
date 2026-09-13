package watcher

import (
	"context"
	"log/slog"
)

// IssueRouter turns a verified, deduped Issue event into whatever
// downstream action a configured gate calls for. Separate from Router
// (which handles AgentSessionEvent) because the two webhook categories
// carry different payload shapes and trigger different behavior — an
// Issue event may match zero gates and do nothing.
type IssueRouter interface {
	RouteIssue(IssueEvent) error
}

// GateRouter checks an Issue event against a configured GateSet and, on
// a genuine transition into a gate's state, delegates the issue to that
// gate's specialist. It never blocks progress itself — see Policy's own
// doc comment — it only starts the specialist's Agent Session and logs
// which policy applies.
type GateRouter struct {
	Gates    GateSet
	Delegate Delegator
	Log      *slog.Logger
}

// RouteIssue implements IssueRouter.
func (r GateRouter) RouteIssue(event IssueEvent) error {
	gate, ok := r.Gates.Match(event.Data.TeamID, event.Data.StateID)
	if !ok {
		return nil
	}
	if !event.TransitionedInto(gate.StateID) {
		return nil
	}

	r.Log.Info("issue entered gate state, delegating",
		"issue_id", event.Data.ID,
		"team_id", event.Data.TeamID,
		"state_id", event.Data.StateID,
		"specialist", gate.Specialist.Name,
		"policy", gate.Policy,
	)

	return r.Delegate.Delegate(context.Background(), event.Data.ID, gate.Specialist)
}
