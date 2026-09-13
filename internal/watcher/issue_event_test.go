package watcher

import "testing"

func TestIssueEvent_TransitionedInto(t *testing.T) {
	tests := []struct {
		name string
		e    IssueEvent
		want bool
	}{
		{
			name: "genuine transition into the target state",
			e: IssueEvent{
				Action:      "update",
				UpdatedFrom: map[string]any{"stateId": "state-todo"},
				Data:        IssueData{StateID: "state-review"},
			},
			want: true,
		},
		{
			name: "update while already in the state — not a transition",
			e: IssueEvent{
				Action:      "update",
				UpdatedFrom: map[string]any{"title": "renamed"},
				Data:        IssueData{StateID: "state-review"},
			},
			want: false,
		},
		{
			name: "update landed on a different state than the one asked about",
			e: IssueEvent{
				Action:      "update",
				UpdatedFrom: map[string]any{"stateId": "state-todo"},
				Data:        IssueData{StateID: "state-done"},
			},
			want: false,
		},
		{
			name: "create action never counts as a transition",
			e: IssueEvent{
				Action: "create",
				Data:   IssueData{StateID: "state-review"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.TransitionedInto("state-review"); got != tt.want {
				t.Errorf("TransitionedInto(%q) = %v, want %v", "state-review", got, tt.want)
			}
		})
	}
}
