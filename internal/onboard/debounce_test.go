package onboard

import (
	"testing"
	"time"
)

func TestDue_FirstCallDueSecondNot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	due, err := Due("mercury@justin", time.Hour)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if !due {
		t.Error("first call: due = false, want true")
	}

	due, err = Due("mercury@justin", time.Hour)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if due {
		t.Error("immediate second call: due = true, want false (within the window)")
	}
}

func TestDue_DueAgainAfterWindowElapses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	due, err := Due("venus@justin", time.Millisecond)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if !due {
		t.Fatal("first call: due = false, want true")
	}

	time.Sleep(5 * time.Millisecond)

	due, err = Due("venus@justin", time.Millisecond)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if !due {
		t.Error("call after the window elapsed: due = false, want true")
	}
}

func TestDue_KeyedPerIdentity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, err := Due("mars@justin", time.Hour); err != nil {
		t.Fatalf("Due: %v", err)
	}
	due, err := Due("jupiter@justin", time.Hour)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if !due {
		t.Error("a different identity should have its own independent debounce marker")
	}
}
