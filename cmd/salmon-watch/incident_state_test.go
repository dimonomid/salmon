package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSnoozeDeadlineUsesWallClock(t *testing.T) {
	state := &snoozeState{
		path:    filepath.Join(t.TempDir(), "state.json"),
		snoozed: make(map[string]snoozeEntry),
	}
	now := time.Now()
	if now == now.Round(0) {
		t.Fatal("test clock does not contain a monotonic reading")
	}

	if err := state.Snooze("server.disk", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	deadline := state.snoozed["server.disk"].SnoozedUntil
	if deadline != deadline.Round(0) {
		t.Fatal("snooze deadline retained a monotonic clock reading")
	}
	if want := now.Round(0).Add(time.Hour); !deadline.Equal(want) {
		t.Fatalf("snooze deadline = %s, want %s", deadline, want)
	}
}
