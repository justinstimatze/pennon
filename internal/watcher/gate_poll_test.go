package watcher

import (
	"context"
	"errors"
	"testing"
)

type fakeQuerier struct {
	byKey map[string][]PolledIssue // key: teamID+"/"+stateID
	err   error
}

func (f fakeQuerier) IssuesInState(_ context.Context, teamID, stateID string) ([]PolledIssue, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byKey[teamID+"/"+stateID], nil
}

func delegatedIssueIDs(f *fakeDelegator) []string {
	ids := make([]string, len(f.calls))
	for i, c := range f.calls {
		ids[i] = c.issueID
	}
	return ids
}

func TestGatePoller_DelegatesUndelegatedIssue(t *testing.T) {
	gates := GateSet{{
		TeamID:     "team-1",
		StateID:    "state-review",
		Specialist: Specialist{Name: "FlintReviewer@justin"},
	}}
	query := fakeQuerier{byKey: map[string][]PolledIssue{
		"team-1/state-review": {{ID: "issue-1"}},
	}}
	delegate := &fakeDelegator{}

	p := GatePoller{Gates: gates, Query: query, Delegate: delegate, Log: discardLogger()}
	if err := p.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	got := delegatedIssueIDs(delegate)
	if len(got) != 1 || got[0] != "issue-1" {
		t.Errorf("delegated = %v, want [issue-1]", got)
	}
}

func TestGatePoller_SkipsAlreadyDelegatedIssue(t *testing.T) {
	gates := GateSet{{
		TeamID:     "team-1",
		StateID:    "state-review",
		Specialist: Specialist{Name: "FlintReviewer@justin"},
	}}
	query := fakeQuerier{byKey: map[string][]PolledIssue{
		"team-1/state-review": {{ID: "issue-1", DelegateName: "FlintReviewer@justin"}},
	}}
	delegate := &fakeDelegator{}

	p := GatePoller{Gates: gates, Query: query, Delegate: delegate, Log: discardLogger()}
	if err := p.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if got := delegatedIssueIDs(delegate); len(got) != 0 {
		t.Errorf("delegated = %v, want none — issue already carries this gate's specialist as delegate", got)
	}
}

func TestGatePoller_RedelegatesWhenDelegateIsSomeoneElse(t *testing.T) {
	gates := GateSet{{
		TeamID:     "team-1",
		StateID:    "state-review",
		Specialist: Specialist{Name: "FlintReviewer@justin"},
	}}
	query := fakeQuerier{byKey: map[string][]PolledIssue{
		"team-1/state-review": {{ID: "issue-1", DelegateName: "mercury@justin"}},
	}}
	delegate := &fakeDelegator{}

	p := GatePoller{Gates: gates, Query: query, Delegate: delegate, Log: discardLogger()}
	if err := p.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	got := delegatedIssueIDs(delegate)
	if len(got) != 1 || got[0] != "issue-1" {
		t.Errorf("delegated = %v, want [issue-1] — delegate belongs to someone other than this gate's specialist", got)
	}
}

func TestGatePoller_OneBadGateDoesNotBlockOthers(t *testing.T) {
	gates := GateSet{
		{TeamID: "team-bad", StateID: "state-x", Specialist: Specialist{Name: "specialist-a"}},
		{TeamID: "team-1", StateID: "state-review", Specialist: Specialist{Name: "FlintReviewer@justin"}},
	}
	query := fakeQuerierMulti{
		"team-bad/state-x":    {err: errors.New("linear is down")},
		"team-1/state-review": {issues: []PolledIssue{{ID: "issue-1"}}},
	}
	delegate := &fakeDelegator{}

	p := GatePoller{Gates: gates, Query: query, Delegate: delegate, Log: discardLogger()}
	err := p.PollOnce(context.Background())
	if err == nil {
		t.Fatal("expected PollOnce to report the failed gate's error")
	}
	got := delegatedIssueIDs(delegate)
	if len(got) != 1 || got[0] != "issue-1" {
		t.Errorf("delegated = %v, want [issue-1] — the good gate should still run despite the bad one", got)
	}
}

type queryResult struct {
	issues []PolledIssue
	err    error
}

type fakeQuerierMulti map[string]queryResult

func (f fakeQuerierMulti) IssuesInState(_ context.Context, teamID, stateID string) ([]PolledIssue, error) {
	r := f[teamID+"/"+stateID]
	return r.issues, r.err
}
