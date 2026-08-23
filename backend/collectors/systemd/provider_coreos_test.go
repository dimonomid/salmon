package systemd

import (
	"errors"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

func TestProviderCoreosSubscriptionLoopStopsOnClose(t *testing.T) {
	unitUpdates := make(chan *UnitUpdate, 2)
	provider := &ProviderCoreos{
		params:     ProviderCoreosParams{Common: ProviderParams{UnitUpdatesChan: unitUpdates}},
		teardownCh: make(chan chan struct{}),
	}
	updates := make(chan map[string]*dbus.UnitStatus, 1)
	errorsCh := make(chan error, 1)
	connectionClosed := make(chan struct{})
	loopDone := make(chan struct{})
	go func() {
		provider.runSubscription(updates, errorsCh, func() { close(connectionClosed) })
		close(loopDone)
	}()

	updates <- map[string]*dbus.UnitStatus{
		"sync.service": {Name: "sync.service", ActiveState: "failed"},
	}
	update := receiveProviderUpdate(t, unitUpdates)
	if got := update.Units["sync.service"]; got == nil || got.Name != "sync.service" || got.State != "failed" {
		t.Fatalf("translated unit = %#v", got)
	}

	wantError := errors.New("subscription failed")
	errorsCh <- wantError
	if got := receiveProviderUpdate(t, unitUpdates).Err; got != wantError {
		t.Fatalf("forwarded error = %v, want %v", got, wantError)
	}

	closeReturned := make(chan struct{})
	go func() {
		provider.Close()
		close(closeReturned)
	}()
	for name, channel := range map[string]<-chan struct{}{
		"connection cleanup": connectionClosed,
		"Close":              closeReturned,
		"subscription loop":  loopDone,
	} {
		select {
		case <-channel:
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", name)
		}
	}
	if _, ok := <-unitUpdates; ok {
		t.Fatal("provider output channel remains open after Close")
	}
}

func receiveProviderUpdate(t *testing.T, updates <-chan *UnitUpdate) *UnitUpdate {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for provider update")
		return nil
	}
}
