package exec_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
	execcollector "github.com/dimonomid/salmon/backend/collectors/exec"
)

func TestCollectorMapsCommandResultsToItems(t *testing.T) {
	tests := []struct {
		name        string
		description string
		command     []string
		conditions  []execcollector.ConfigCondition
		wantState   salmon.ItemState
		wantText    string
	}{
		{
			name:        "matching exit code",
			description: "probe",
			command:     []string{"sh", "-c", "exit 7"},
			conditions: []execcollector.ConfigCondition{
				{ExitCode: "0", Result: salmon.ItemStateOK},
				{ExitCode: "7", Result: salmon.ItemStateWarning},
			},
			wantState: salmon.ItemStateWarning,
			wantText:  "exit code: 7, applied condition #1",
		},
		{
			name:       "unmatched exit code defaults to error",
			command:    []string{"sh", "-c", "exit 9"},
			conditions: []execcollector.ConfigCondition{{ExitCode: "0", Result: salmon.ItemStateOK}},
			wantState:  salmon.ItemStateError,
			wantText:   "did not find matching condition",
		},
		{
			name:       "command start failure is an incident",
			command:    []string{"/a/salmon-command-that-does-not-exist"},
			conditions: []execcollector.ConfigCondition{{Result: salmon.ItemStateOK}},
			wantState:  salmon.ItemStateError,
			wantText:   "failed to start command",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updates := make(chan *collectors.Update, 1)
			collector, err := execcollector.NewCollector(execcollector.CollectorParams{
				Common: collectors.Params{ID: "check", UpdatesChan: updates},
				Config: execcollector.Config{
					Description:               test.description,
					Command:                   test.command,
					PollInterval:              time.Hour,
					PollIntervalWhenUnhealthy: time.Hour,
					Conditions:                test.conditions,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(collector.Close)

			update := receiveUpdate(t, updates)
			got := update.Items["check.exec_result"]
			if got == nil {
				t.Fatalf("update = %#v, want check.exec_result", update)
			}
			if got.State != test.wantState {
				t.Errorf("state = %q, want %q", got.State, test.wantState)
			}
			if !strings.Contains(got.Details, test.wantText) {
				t.Errorf("details = %q, want it to contain %q", got.Details, test.wantText)
			}
			if test.description != "" && !strings.HasPrefix(got.Details, test.description+": ") {
				t.Errorf("details = %q, want description prefix %q", got.Details, test.description+": ")
			}
			if strings.HasPrefix(got.Details, ":") {
				t.Errorf("details without description have a leading colon: %q", got.Details)
			}
		})
	}
}

func TestCollectorCloseCancelsAStuckCommand(t *testing.T) {
	updates := make(chan *collectors.Update)
	collector, err := execcollector.NewCollector(execcollector.CollectorParams{
		Common: collectors.Params{ID: "check", UpdatesChan: updates},
		Config: execcollector.Config{
			Command:                   []string{"sh", "-c", "sleep 30"},
			PollInterval:              time.Hour,
			PollIntervalWhenUnhealthy: time.Hour,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		collector.Close()
		collector.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not cancel the active command")
	}
}

func receiveUpdate(t *testing.T, updates <-chan *collectors.Update) *collectors.Update {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for collector update")
		return nil
	}
}
