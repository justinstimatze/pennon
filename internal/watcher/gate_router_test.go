package watcher

import (
	"context"
	"errors"
	"testing"
)

type fakeDelegator struct {
	calls []struct {
		issueID    string
		specialist Specialist
	}
	err error
}

func (f *fakeDelegator) Delegate(_ context.Context, issueID string, specialist Specialist) error {
	f.calls = append(f.calls, struct {
		issueID    string
		specialist Specialist
	}{issueID, specialist})
	return f.err
}

func TestGateRouter_RouteIssue(t *testing.T) {
	gates := GateSet{
		{TeamID: "team-1", StateID: "state-review", Policy: PolicyAdvisory, Specialist: Specialist{Name: "Code Reviewer Flint"}},
	}

	t.Run("transition into a configured gate delegates", func(t *testing.T) {
		delegator := &fakeDelegator{}
		router := GateRouter{Gates: gates, Delegate: delegator, Log: discardLogger()}

		event := IssueEvent{
			Action:      "update",
			UpdatedFrom: map[string]any{"stateId": "state-todo"},
			Data:        IssueData{ID: "issue-1", TeamID: "team-1", StateID: "state-review"},
		}
		if err := router.RouteIssue(event); err != nil {
			t.Fatalf("RouteIssue: %v", err)
		}
		if len(delegator.calls) != 1 {
			t.Fatalf("delegated %d times, want 1", len(delegator.calls))
		}
		if delegator.calls[0].issueID != "issue-1" || delegator.calls[0].specialist.Name != "Code Reviewer Flint" {
			t.Errorf("delegate call = %+v, missing expected fields", delegator.calls[0])
		}
	})

	t.Run("no gate configured for the team/state does nothing", func(t *testing.T) {
		delegator := &fakeDelegator{}
		router := GateRouter{Gates: gates, Delegate: delegator, Log: discardLogger()}

		event := IssueEvent{
			Action:      "update",
			UpdatedFrom: map[string]any{"stateId": "state-todo"},
			Data:        IssueData{ID: "issue-2", TeamID: "team-9", StateID: "state-review"},
		}
		if err := router.RouteIssue(event); err != nil {
			t.Fatalf("RouteIssue: %v", err)
		}
		if len(delegator.calls) != 0 {
			t.Errorf("delegated %d times, want 0 for an unconfigured team/state", len(delegator.calls))
		}
	})

	t.Run("update that isn't a transition into the gate state does nothing", func(t *testing.T) {
		delegator := &fakeDelegator{}
		router := GateRouter{Gates: gates, Delegate: delegator, Log: discardLogger()}

		event := IssueEvent{
			Action:      "update",
			UpdatedFrom: map[string]any{"title": "renamed"},
			Data:        IssueData{ID: "issue-3", TeamID: "team-1", StateID: "state-review"},
		}
		if err := router.RouteIssue(event); err != nil {
			t.Fatalf("RouteIssue: %v", err)
		}
		if len(delegator.calls) != 0 {
			t.Errorf("delegated %d times, want 0 for a non-transition update", len(delegator.calls))
		}
	})

	t.Run("delegator error surfaces", func(t *testing.T) {
		delegator := &fakeDelegator{err: errors.New("mint failed")}
		router := GateRouter{Gates: gates, Delegate: delegator, Log: discardLogger()}

		event := IssueEvent{
			Action:      "update",
			UpdatedFrom: map[string]any{"stateId": "state-todo"},
			Data:        IssueData{ID: "issue-4", TeamID: "team-1", StateID: "state-review"},
		}
		if err := router.RouteIssue(event); err == nil {
			t.Fatal("expected an error when the delegator fails")
		}
	})
}
