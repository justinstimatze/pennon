package watcher

import "testing"

func TestGateSet_Match(t *testing.T) {
	gates := GateSet{
		{TeamID: "team-1", StateID: "state-review", Policy: PolicyAdvisory, Specialist: Specialist{Name: "Code Reviewer Flint"}},
		{TeamID: "team-2", StateID: "state-security", Policy: PolicyVeto, Specialist: Specialist{Name: "Code Reviewer Jasper"}},
	}

	t.Run("matches by team and state together", func(t *testing.T) {
		gate, ok := gates.Match("team-1", "state-review")
		if !ok {
			t.Fatal("expected a match")
		}
		if gate.Specialist.Name != "Code Reviewer Flint" {
			t.Errorf("matched specialist = %q, want Code Reviewer Flint", gate.Specialist.Name)
		}
	})

	t.Run("right team, wrong state does not match", func(t *testing.T) {
		if _, ok := gates.Match("team-1", "state-security"); ok {
			t.Error("expected no match")
		}
	})

	t.Run("right state, wrong team does not match", func(t *testing.T) {
		if _, ok := gates.Match("team-2", "state-review"); ok {
			t.Error("expected no match")
		}
	})

	t.Run("unconfigured pair does not match", func(t *testing.T) {
		if _, ok := gates.Match("team-9", "state-9"); ok {
			t.Error("expected no match")
		}
	})
}
