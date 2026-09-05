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
		params: ProviderCoreosParams{Common: ProviderParams{UnitUpdatesChan: unitUpdates}},
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
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
		"sync.service":    {Name: "sync.service", ActiveState: "activating", SubState: "auto-restart"},
		"queued.service":  {Name: "queued.service", ActiveState: "activating", SubState: "auto-restart-queued"},
		"healthy.service": {Name: "healthy.service", ActiveState: "active", SubState: "running"},
	}
	update := receiveProviderUpdate(t, unitUpdates)
	if got := update.Units["sync.service"]; got == nil || got.Name != "sync.service" || got.State != "activating" || got.SubState != "auto-restart" {
		t.Fatalf("translated unit = %#v", got)
	}
	if got := update.Units["queued.service"]; got == nil || got.SubState != "auto-restart-queued" {
		t.Fatalf("translated queued unit = %#v", got)
	}
	if got := update.Units["healthy.service"]; got == nil || got.State != "active" || got.SubState != "running" {
		t.Fatalf("translated healthy unit = %#v", got)
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

func TestProviderCoreosCloseInterruptsBlockedUpdate(t *testing.T) {
	unitUpdates := make(chan *UnitUpdate)
	provider := &ProviderCoreos{
		params: ProviderCoreosParams{Common: ProviderParams{UnitUpdatesChan: unitUpdates}},
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	updates := make(chan map[string]*dbus.UnitStatus)
	errorsCh := make(chan error)
	connectionClosed := make(chan struct{})
	go provider.runSubscription(updates, errorsCh, func() { close(connectionClosed) })

	// Once this send completes, the provider has accepted the subscription
	// update and is trying to forward it to the unread output channel.
	updates <- map[string]*dbus.UnitStatus{
		"blocked.service": {Name: "blocked.service", ActiveState: "failed"},
	}

	closeReturned := make(chan struct{})
	go func() {
		provider.Close()
		provider.Close()
		close(closeReturned)
	}()
	for name, channel := range map[string]<-chan struct{}{
		"connection cleanup": connectionClosed,
		"Close":              closeReturned,
	} {
		select {
		case <-channel:
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", name)
		}
	}
	if _, ok := <-unitUpdates; ok {
		t.Fatal("provider output channel remains open after blocked-send shutdown")
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
