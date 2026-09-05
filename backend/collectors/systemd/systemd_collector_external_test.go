package systemd_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
	"github.com/dimonomid/salmon/backend/collectors/systemd"
	"github.com/dimonomid/salmon/logs"
)

var testLogger = logs.NewLogger(logs.LoggerParams{Clock: clock.New()})

type controlledProvider struct {
	updates chan<- *systemd.UnitUpdate
	closed  chan struct{}
}

func (p *controlledProvider) Close() {
	close(p.updates)
	close(p.closed)
}

func TestCollectorAppliesOrderedRulesAndReportsRemovedUnits(t *testing.T) {
	updates := make(chan *collectors.Update, 4)
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: updates},
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{
			{
				Names:      []string{"important.service", "another-important.service"},
				Conditions: []systemd.ConfigCondition{{State: "active", Result: salmon.ItemStateOK}, {Result: salmon.ItemStateError}},
			},
			{
				Names:      []string{"ignored.service"},
				Conditions: []systemd.ConfigCondition{{Result: salmon.ItemStateOK}},
			},
			{
				Type: "service",
				Conditions: []systemd.ConfigCondition{
					{SubStateContains: "auto-restart", Result: salmon.ItemStateWarning},
					{State: "active", Result: salmon.ItemStateOK},
					{State: "activating", Result: salmon.ItemStateOK},
					{Result: salmon.ItemStateWarning},
				},
			},
		}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			provider = &controlledProvider{updates: params.UnitUpdatesChan, closed: make(chan struct{})}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	provider.updates <- &systemd.UnitUpdate{Units: map[string]*systemd.Unit{
		"failed.service":            {Name: "failed.service", State: "failed"},
		"active.service":            {Name: "active.service", State: "active"},
		"restarting.service":        {Name: "restarting.service", State: "activating", SubState: "auto-restart"},
		"restart-queued.service":    {Name: "restart-queued.service", State: "activating", SubState: "auto-restart-queued"},
		"restart-wait.service":      {Name: "restart-wait.service", State: "inactive", SubState: "dead-before-auto-restart"},
		"recovering.service":        {Name: "recovering.service", State: "activating", SubState: "auto-restart"},
		"starting.service":          {Name: "starting.service", State: "activating", SubState: "start"},
		"another-important.service": {Name: "another-important.service", State: "active"},
		"socket.socket":             {Name: "socket.socket", State: "failed"},
	}}
	first := receiveSystemdUpdate(t, updates)
	assertSystemdItem(t, first, "services.failed.service", salmon.ItemStateWarning)
	assertSystemdItem(t, first, "services.active.service", salmon.ItemStateOK)
	assertSystemdItem(t, first, "services.restarting.service", salmon.ItemStateWarning)
	assertSystemdItem(t, first, "services.restart-queued.service", salmon.ItemStateWarning)
	assertSystemdItem(t, first, "services.restart-wait.service", salmon.ItemStateWarning)
	assertSystemdItem(t, first, "services.recovering.service", salmon.ItemStateWarning)
	assertSystemdItem(t, first, "services.starting.service", salmon.ItemStateOK)
	assertSystemdItem(t, first, "services.important.service", salmon.ItemStateError)
	assertSystemdItem(t, first, "services.another-important.service", salmon.ItemStateOK)
	assertSystemdItem(t, first, "services.ignored.service", salmon.ItemStateOK)
	assertSystemdDetails(t, first, "services.failed.service", "Unit failed.service is failed")
	assertSystemdDetails(t, first, "services.restarting.service", "Unit restarting.service is activating (auto-restart)")
	assertSystemdDetails(t, first, "services.restart-queued.service", "Unit restart-queued.service is activating (auto-restart-queued)")
	assertSystemdDetails(t, first, "services.restart-wait.service", "Unit restart-wait.service is inactive (dead-before-auto-restart)")
	assertSystemdDetails(t, first, "services.recovering.service", "Unit recovering.service is activating (auto-restart)")
	assertSystemdDetails(t, first, "services.starting.service", "Unit starting.service is activating (start)")
	assertSystemdDetails(t, first, "services.another-important.service", "Unit another-important.service is active")
	assertSystemdDetails(t, first, "services.important.service", "Unit important.service was not reported by systemd")
	if _, exists := first.Items["services.socket.socket"]; exists {
		t.Error("unmatched socket unit was published")
	}

	provider.updates <- &systemd.UnitUpdate{Units: map[string]*systemd.Unit{"failed.service": nil}}
	removed := receiveSystemdUpdate(t, updates)
	assertSystemdItem(t, removed, "services.failed.service", salmon.ItemStateWarning)

	collector.Close()
	select {
	case <-provider.closed:
	case <-time.After(time.Second):
		t.Fatal("collector did not close its provider")
	}
}

func TestCollectorResolvePolicyRequiresContinuousRecovery(t *testing.T) {
	updates := make(chan *collectors.Update, 8)
	mockClock := clock.NewMock()
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: updates},
		Clock:  mockClock,
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
			Type: "service",
			Conditions: []systemd.ConfigCondition{
				{
					SubStateContains: "auto-restart",
					Result:           salmon.ItemStateWarning,
					Resolve: &systemd.ConfigResolve{
						After:  5 * time.Second,
						States: []systemd.UnitState{"active", "inactive"},
					},
				},
				{State: "active", Result: salmon.ItemStateOK},
				{State: "inactive", Result: salmon.ItemStateOK},
				{State: "activating", Result: salmon.ItemStateOK},
				{Result: salmon.ItemStateWarning},
			},
		}}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			provider = &controlledProvider{updates: params.UnitUpdatesChan, closed: make(chan struct{})}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(collector.Close)

	provider.updates <- systemdUnitUpdate("flapping.service", "activating", "auto-restart")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.flapping.service", salmon.ItemStateWarning)

	// A restart loop can spend longer than resolve.after in activating (start)
	// before failing again. Although that transitional state is normally OK, it
	// must not resolve an existing auto-restart incident.
	provider.updates <- systemdUnitUpdate("flapping.service", "activating", "start")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("transitional barrier")}
	receiveSystemdUpdate(t, updates)
	mockClock.Add(10 * time.Second)
	assertNoSystemdUpdate(t, updates)
	provider.updates <- systemdUnitUpdate("flapping.service", "activating", "auto-restart")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.flapping.service", salmon.ItemStateWarning)

	provider.updates <- systemdUnitUpdate("flapping.service", "active", "running")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("healthy barrier")}
	if update := receiveSystemdUpdate(t, updates); update.Err == nil || !strings.Contains(update.Err.Error(), "healthy barrier") {
		t.Fatalf("barrier update = %#v, want healthy barrier error", update)
	}
	assertNoSystemdUpdate(t, updates)

	mockClock.Add(3 * time.Second)
	provider.updates <- systemdUnitUpdate("flapping.service", "active", "exited")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("updated healthy barrier")}
	if update := receiveSystemdUpdate(t, updates); update.Err == nil || !strings.Contains(update.Err.Error(), "updated healthy barrier") {
		t.Fatalf("barrier update = %#v, want updated healthy barrier error", update)
	}
	mockClock.Add(2 * time.Second)
	recovered := receiveSystemdUpdate(t, updates)
	assertSystemdItem(t, recovered, "services.flapping.service", salmon.ItemStateOK)
	assertSystemdDetails(t, recovered, "services.flapping.service", "Unit flapping.service is active (exited)")

	// A new unhealthy update cancels a pending recovery. The next healthy
	// update must then remain healthy for the full resolve.after duration.
	provider.updates <- systemdUnitUpdate("flapping.service", "activating", "auto-restart")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.flapping.service", salmon.ItemStateWarning)
	provider.updates <- systemdUnitUpdate("flapping.service", "active", "running")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("second healthy barrier")}
	receiveSystemdUpdate(t, updates)
	mockClock.Add(2 * time.Second)
	provider.updates <- systemdUnitUpdate("flapping.service", "activating", "auto-restart-queued")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.flapping.service", salmon.ItemStateWarning)
	mockClock.Add(5 * time.Second)
	assertNoSystemdUpdate(t, updates)

	provider.updates <- systemdUnitUpdate("flapping.service", "active", "running")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("final healthy barrier")}
	receiveSystemdUpdate(t, updates)
	mockClock.Add(5 * time.Second)
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.flapping.service", salmon.ItemStateOK)

	// resolve is attached to the condition that starts an incident. A
	// warning from the fallback condition therefore resolves immediately.
	provider.updates <- systemdUnitUpdate("failed.service", "failed", "failed")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.failed.service", salmon.ItemStateWarning)
	provider.updates <- systemdUnitUpdate("failed.service", "active", "running")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.failed.service", salmon.ItemStateOK)
}

func TestCollectorResolvePolicyUsesConfiguredStates(t *testing.T) {
	updates := make(chan *collectors.Update, 3)
	mockClock := clock.NewMock()
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: updates},
		Clock:  mockClock,
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
			Type: "service",
			Conditions: []systemd.ConfigCondition{
				{
					SubStateContains: "auto-restart",
					Result:           salmon.ItemStateWarning,
					Resolve: &systemd.ConfigResolve{
						After:  time.Second,
						States: []systemd.UnitState{"activating"},
					},
				},
				{State: "activating", Result: salmon.ItemStateOK},
			},
		}}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			provider = &controlledProvider{updates: params.UnitUpdatesChan, closed: make(chan struct{})}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(collector.Close)

	provider.updates <- systemdUnitUpdate("custom-recovery.service", "activating", "auto-restart")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.custom-recovery.service", salmon.ItemStateWarning)
	provider.updates <- systemdUnitUpdate("custom-recovery.service", "activating", "start")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("configured-state barrier")}
	receiveSystemdUpdate(t, updates)
	mockClock.Add(time.Second)
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.custom-recovery.service", salmon.ItemStateOK)
}

func TestCollectorResolvePolicyDisallowedOKStateResetsTimer(t *testing.T) {
	updates := make(chan *collectors.Update, 4)
	mockClock := clock.NewMock()
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: updates},
		Clock:  mockClock,
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
			Type: "service",
			Conditions: []systemd.ConfigCondition{
				{
					SubStateContains: "auto-restart",
					Result:           salmon.ItemStateWarning,
					Resolve: &systemd.ConfigResolve{
						After:  5 * time.Second,
						States: []systemd.UnitState{"active"},
					},
				},
				{State: "active", Result: salmon.ItemStateOK},
				{State: "activating", Result: salmon.ItemStateOK},
			},
		}}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			provider = &controlledProvider{updates: params.UnitUpdatesChan, closed: make(chan struct{})}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(collector.Close)

	provider.updates <- systemdUnitUpdate("reset-recovery.service", "activating", "auto-restart")
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.reset-recovery.service", salmon.ItemStateWarning)

	provider.updates <- systemdUnitUpdate("reset-recovery.service", "active", "running")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("first allowed-state barrier")}
	receiveSystemdUpdate(t, updates)
	mockClock.Add(3 * time.Second)

	// activating is OK according to the rules, but it is not in resolve.states.
	// It must cancel the active-state recovery timer instead of resolving the
	// incident or allowing that timer to finish in the background.
	provider.updates <- systemdUnitUpdate("reset-recovery.service", "activating", "start")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("disallowed-state barrier")}
	receiveSystemdUpdate(t, updates)
	mockClock.Add(5 * time.Second)
	assertNoSystemdUpdate(t, updates)

	provider.updates <- systemdUnitUpdate("reset-recovery.service", "active", "running")
	provider.updates <- &systemd.UnitUpdate{Err: errors.New("second allowed-state barrier")}
	receiveSystemdUpdate(t, updates)
	mockClock.Add(4 * time.Second)
	assertNoSystemdUpdate(t, updates)
	mockClock.Add(time.Second)
	assertSystemdItem(t, receiveSystemdUpdate(t, updates), "services.reset-recovery.service", salmon.ItemStateOK)
}

func TestCollectorForwardsProviderErrors(t *testing.T) {
	updates := make(chan *collectors.Update, 1)
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: updates},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			provider = &controlledProvider{updates: params.UnitUpdatesChan, closed: make(chan struct{})}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(collector.Close)

	provider.updates <- &systemd.UnitUpdate{Err: errors.New("dbus unavailable")}
	update := receiveSystemdUpdate(t, updates)
	if update.Err == nil || update.Err.Error() != "got error from systemd conn: dbus unavailable" {
		t.Fatalf("error = %v, want wrapped provider error", update.Err)
	}
}

func TestCollectorDefaultsUnmatchedConditionToError(t *testing.T) {
	updates := make(chan *collectors.Update, 1)
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: updates},
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
			Names:      []string{"unmatched.service"},
			Conditions: []systemd.ConfigCondition{{State: "active", Result: salmon.ItemStateOK}},
		}}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			provider = &controlledProvider{updates: params.UnitUpdatesChan, closed: make(chan struct{})}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(collector.Close)

	provider.updates <- &systemd.UnitUpdate{Units: map[string]*systemd.Unit{
		"unmatched.service": {Name: "unmatched.service", State: "inactive"},
	}}
	update := receiveSystemdUpdate(t, updates)
	assertSystemdItem(t, update, "services.unmatched.service", salmon.ItemStateError)
	assertSystemdDetails(t, update, "services.unmatched.service", "Unit unmatched.service is inactive")
}

func TestCollectorRejectsInvalidResultsBeforeStartingProvider(t *testing.T) {
	providerCalled := false
	_, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: make(chan *collectors.Update)},
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
			Type: "service", Conditions: []systemd.ConfigCondition{{Result: "banana"}},
		}}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			providerCalled = true
			return nil, nil
		},
	})
	if err == nil {
		t.Fatal("invalid result was accepted")
	}
	if providerCalled {
		t.Fatal("provider was started before configuration validation")
	}
}

func TestCollectorRejectsBothSubStateMatchersBeforeStartingProvider(t *testing.T) {
	providerCalled := false
	_, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: make(chan *collectors.Update)},
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
			Type: "service",
			Conditions: []systemd.ConfigCondition{{
				SubState:         "auto-restart",
				SubStateContains: "auto-restart",
				Result:           salmon.ItemStateWarning,
			}},
		}}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			providerCalled = true
			return nil, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "must not specify both subState and subStateContains") {
		t.Fatalf("error = %v, want mutually exclusive matcher error", err)
	}
	if providerCalled {
		t.Fatal("provider was started before configuration validation")
	}
}

func TestCollectorRejectsInvalidResolveBeforeStartingProvider(t *testing.T) {
	tests := []struct {
		name      string
		condition systemd.ConfigCondition
		want      string
	}{
		{
			name:      "negative",
			condition: systemd.ConfigCondition{Result: salmon.ItemStateWarning, Resolve: &systemd.ConfigResolve{After: -time.Second, States: []systemd.UnitState{"active"}}},
			want:      "resolve.after must be positive",
		},
		{
			name:      "no states",
			condition: systemd.ConfigCondition{Result: salmon.ItemStateWarning, Resolve: &systemd.ConfigResolve{After: time.Second}},
			want:      "resolve.states must not be empty",
		},
		{
			name:      "empty state",
			condition: systemd.ConfigCondition{Result: salmon.ItemStateWarning, Resolve: &systemd.ConfigResolve{After: time.Second, States: []systemd.UnitState{"active", ""}}},
			want:      "resolve state #1 must not be empty",
		},
		{
			name:      "duplicate state",
			condition: systemd.ConfigCondition{Result: salmon.ItemStateWarning, Resolve: &systemd.ConfigResolve{After: time.Second, States: []systemd.UnitState{"active", "active"}}},
			want:      "resolve contains duplicate state",
		},
		{
			name:      "ok result",
			condition: systemd.ConfigCondition{Result: salmon.ItemStateOK, Resolve: &systemd.ConfigResolve{After: time.Second, States: []systemd.UnitState{"active"}}},
			want:      "requires a non-OK result",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerCalled := false
			_, err := systemd.NewCollector(systemd.CollectorParams{
				Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: make(chan *collectors.Update)},
				Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
					Type:       "service",
					Conditions: []systemd.ConfigCondition{test.condition},
				}}},
				ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
					providerCalled = true
					return nil, nil
				},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text %q", err, test.want)
			}
			if providerCalled {
				t.Fatal("provider was started before configuration validation")
			}
		})
	}
}

func TestCollectorRejectsInvalidNamesBeforeStartingProvider(t *testing.T) {
	tests := []struct {
		name  string
		names []string
	}{
		{name: "empty", names: []string{""}},
		{name: "duplicate", names: []string{"same.service", "same.service"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerCalled := false
			_, err := systemd.NewCollector(systemd.CollectorParams{
				Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: make(chan *collectors.Update)},
				Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
					Names:      test.names,
					Conditions: []systemd.ConfigCondition{{Result: salmon.ItemStateOK}},
				}}},
				ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
					providerCalled = true
					return nil, nil
				},
			})
			if err == nil {
				t.Fatal("invalid names were accepted")
			}
			if providerCalled {
				t.Fatal("provider was started before configuration validation")
			}
		})
	}
}

func TestCollectorReturnsProviderStartupError(t *testing.T) {
	want := errors.New("no system bus")
	_, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: make(chan *collectors.Update)},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			return nil, want
		},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestCollectorCloseCompletesWhileCoreOutputIsBlocked(t *testing.T) {
	// Leave the core-facing channel unbuffered and unread. Once the collector
	// receives a provider update, it will block trying to publish it.
	coreUpdates := make(chan *collectors.Update)
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", Logger: testLogger, UpdatesChan: coreUpdates},
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{{
			Type: "service", Conditions: []systemd.ConfigCondition{{Result: salmon.ItemStateError}},
		}}},
		ProviderFactory: func(params systemd.ProviderParams) (systemd.Provider, error) {
			provider = &controlledProvider{updates: params.UnitUpdatesChan, closed: make(chan struct{})}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Closing the provider preserves this accepted update for the collector to
	// drain. Whether it has already reached the blocked core send or receives
	// the update during Close, shutdown must still complete.
	provider.updates <- &systemd.UnitUpdate{Units: map[string]*systemd.Unit{
		"blocked.service": {Name: "blocked.service", State: "failed"},
	}}

	closed := make(chan struct{})
	go func() {
		collector.Close()
		collector.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("collector shutdown blocked on the unread core update channel")
	}
}

func receiveSystemdUpdate(t *testing.T, updates <-chan *collectors.Update) *collectors.Update {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for systemd collector update")
		return nil
	}
}

func assertNoSystemdUpdate(t *testing.T, updates <-chan *collectors.Update) {
	t.Helper()
	select {
	case update := <-updates:
		t.Fatalf("unexpected systemd collector update: %#v", update)
	default:
	}
}

func systemdUnitUpdate(name string, state systemd.UnitState, subState string) *systemd.UnitUpdate {
	return &systemd.UnitUpdate{Units: map[string]*systemd.Unit{
		name: {Name: name, State: state, SubState: subState},
	}}
}

func assertSystemdItem(t *testing.T, update *collectors.Update, key salmon.ItemKey, state salmon.ItemState) {
	t.Helper()
	item := update.Items[key]
	if item == nil {
		t.Errorf("update does not contain %q", key)
		return
	}
	if item.State != state {
		t.Errorf("%s state = %q, want %q", key, item.State, state)
	}
}

func assertSystemdDetails(t *testing.T, update *collectors.Update, key salmon.ItemKey, details string) {
	t.Helper()
	item := update.Items[key]
	if item == nil {
		t.Errorf("update does not contain %q", key)
		return
	}
	if item.Details != details {
		t.Errorf("%s details = %q, want %q", key, item.Details, details)
	}
}
