package systemd_test

import (
	"errors"
	"testing"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
	"github.com/dimonomid/salmon/backend/collectors/systemd"
)

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
		Common: collectors.Params{ID: "services", UpdatesChan: updates},
		Config: systemd.Config{UnitRules: []systemd.ConfigUnitRule{
			{
				Name:       "important.service",
				Conditions: []systemd.ConfigCondition{{State: "active", Result: salmon.ItemStateOK}, {Result: salmon.ItemStateError}},
			},
			{
				Name:       "ignored.service",
				Conditions: []systemd.ConfigCondition{{Result: salmon.ItemStateOK}},
			},
			{
				Type:       "service",
				Conditions: []systemd.ConfigCondition{{State: "active", Result: salmon.ItemStateOK}, {Result: salmon.ItemStateWarning}},
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
		"failed.service": {Name: "failed.service", State: "failed"},
		"active.service": {Name: "active.service", State: "active"},
		"socket.socket":  {Name: "socket.socket", State: "failed"},
	}}
	first := receiveSystemdUpdate(t, updates)
	assertSystemdItem(t, first, "services.failed.service", salmon.ItemStateWarning)
	assertSystemdItem(t, first, "services.active.service", salmon.ItemStateOK)
	assertSystemdItem(t, first, "services.important.service", salmon.ItemStateError)
	assertSystemdItem(t, first, "services.ignored.service", salmon.ItemStateOK)
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

func TestCollectorForwardsProviderErrors(t *testing.T) {
	updates := make(chan *collectors.Update, 1)
	var provider *controlledProvider
	collector, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", UpdatesChan: updates},
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

func TestCollectorRejectsInvalidResultsBeforeStartingProvider(t *testing.T) {
	providerCalled := false
	_, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", UpdatesChan: make(chan *collectors.Update)},
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

func TestCollectorReturnsProviderStartupError(t *testing.T) {
	want := errors.New("no system bus")
	_, err := systemd.NewCollector(systemd.CollectorParams{
		Common: collectors.Params{ID: "services", UpdatesChan: make(chan *collectors.Update)},
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
		Common: collectors.Params{ID: "services", UpdatesChan: coreUpdates},
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
