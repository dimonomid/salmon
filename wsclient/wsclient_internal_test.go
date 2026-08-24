package wsclient

import (
	"testing"
	"time"

	"github.com/dimonomid/salmon"
)

func TestSendOngoingIncidentsWaitsForCapacity(t *testing.T) {
	first := &salmon.Notification{}
	second := &salmon.Notification{}
	notifications := make(chan *salmon.Notification, 1)
	notifications <- first
	client := &WSClient{
		params:    Params{Config: ConfigServer{ID: "test"}, OngoingIncidentsCh: notifications},
		interrupt: make(chan struct{}),
	}
	result := make(chan bool, 1)
	go func() { result <- client.sendOngoingIncidents(second) }()

	if got := <-notifications; got != first {
		t.Fatalf("first notification = %p, want %p", got, first)
	}
	select {
	case sent := <-result:
		if !sent {
			t.Fatal("incident send was canceled")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocking incident send did not complete after capacity became available")
	}
	if got := <-notifications; got != second {
		t.Fatalf("second notification = %p, want %p", got, second)
	}
}

func TestSendOngoingIncidentsUnblocksWhenInterrupted(t *testing.T) {
	notifications := make(chan *salmon.Notification, 1)
	notifications <- &salmon.Notification{}
	client := &WSClient{
		params:    Params{Config: ConfigServer{ID: "test"}, OngoingIncidentsCh: notifications},
		interrupt: make(chan struct{}),
	}
	result := make(chan bool, 1)
	go func() { result <- client.sendOngoingIncidents(&salmon.Notification{}) }()

	close(client.interrupt)
	select {
	case sent := <-result:
		if sent {
			t.Fatal("incident was reported sent after interruption")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocking incident send did not unblock after interruption")
	}
}

func TestSendConnectionEventWaitsForCapacity(t *testing.T) {
	first := ConnectionEvent{EventKind: EventKindConnected}
	second := ConnectionEvent{EventKind: EventKindDisconnected}
	events := make(chan ConnectionEvent, 1)
	events <- first
	client := &WSClient{
		params:    Params{Config: ConfigServer{ID: "test"}, ConnectionEventCh: events},
		interrupt: make(chan struct{}),
	}
	result := make(chan bool, 1)
	go func() { result <- client.sendConnectionEvent(second) }()

	if got := <-events; got != first {
		t.Fatalf("first connection event = %#v, want %#v", got, first)
	}
	select {
	case sent := <-result:
		if !sent {
			t.Fatal("connection event send was canceled")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocking connection event send did not complete after capacity became available")
	}
	if got := <-events; got != second {
		t.Fatalf("second connection event = %#v, want %#v", got, second)
	}
}

func TestSendConnectionEventUnblocksWhenInterrupted(t *testing.T) {
	events := make(chan ConnectionEvent, 1)
	events <- ConnectionEvent{EventKind: EventKindConnected}
	client := &WSClient{
		params:    Params{Config: ConfigServer{ID: "test"}, ConnectionEventCh: events},
		interrupt: make(chan struct{}),
	}
	result := make(chan bool, 1)
	go func() {
		result <- client.sendConnectionEvent(ConnectionEvent{EventKind: EventKindDisconnected})
	}()

	close(client.interrupt)
	select {
	case sent := <-result:
		if sent {
			t.Fatal("connection event was reported sent after interruption")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocking connection event send did not unblock after interruption")
	}
}
