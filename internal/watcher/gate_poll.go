package watcher

import (
	"context"
	"fmt"
	"log/slog"
)

// PolledIssue is the lightweight view of an issue a poll cycle needs:
// enough to match it against a gate and tell whether it's already been
// handed to that gate's specialist.
type PolledIssue struct {
	ID           string
	DelegateName string // empty when the issue has no delegate yet
}

// IssueQuerier looks up every issue currently sitting in a given team's
// workflow state — the poll-based stand-in for a webhook's own
// (team, state) pair, used by GatePoller to find issues a gate should
// act on without Linear ever pushing anything.
type IssueQuerier interface {
	IssuesInState(ctx context.Context, teamID, stateID string) ([]PolledIssue, error)
}

// GatePoller is the poll-based alternative to GateRouter: instead of
// reacting to an Issue webhook's genuine state transition, it
// periodically asks Linear which issues currently sit in each configured
// gate's state and delegates any that aren't already delegated to that
// gate's specialist. See docs/ARCHITECTURE.md "Polling instead of the
// watcher" for why this exists — it trades near-instant delegation for
// not needing anything reachable from the public internet.
//
// Idempotent by design, not by tracking state across cycles: an issue
// already delegated to the right specialist is left alone every time
// PollOnce sees it again, so running this on a timer never re-delegates
// the same issue on every tick the way a naive "issue is in this state"
// check would.
type GatePoller struct {
	Gates    GateSet
	Query    IssueQuerier
	Delegate Delegator
	Log      *slog.Logger
}

// PollOnce checks every configured gate once. It logs and continues past
// a failure on one gate or one issue rather than aborting the whole
// cycle — a scheduler (cron, a systemd timer, a loop) is expected to
// call this repeatedly, so one bad issue shouldn't cost every other gate
// this cycle's delegation.
func (p GatePoller) PollOnce(ctx context.Context) error {
	var errCount int
	for _, gate := range p.Gates {
		issues, err := p.Query.IssuesInState(ctx, gate.TeamID, gate.StateID)
		if err != nil {
			errCount++
			p.Log.Error("poll: query issues for gate failed",
				"team_id", gate.TeamID, "state_id", gate.StateID, "error", err)
			continue
		}
		for _, issue := range issues {
			if issue.DelegateName == gate.Specialist.Name {
				continue
			}
			p.Log.Info("polled issue sitting in gate state, delegating",
				"issue_id", issue.ID,
				"team_id", gate.TeamID,
				"state_id", gate.StateID,
				"specialist", gate.Specialist.Name,
				"policy", gate.Policy,
			)
			if err := p.Delegate.Delegate(ctx, issue.ID, gate.Specialist); err != nil {
				errCount++
				p.Log.Error("poll: delegate failed",
					"issue_id", issue.ID, "specialist", gate.Specialist.Name, "error", err)
			}
		}
	}
	if errCount > 0 {
		return fmt.Errorf("watcher: %d error(s) during poll cycle, see logged detail", errCount)
	}
	return nil
}
