package core

import (
	"testing"
	"time"

	"github.com/dimonomid/salmon"
)

type namedMessenger string

func (m namedMessenger) String() string { return string(m) }

func TestSendMessengerNotificationWaitsForCapacity(t *testing.T) {
	first := &salmon.Notification{}
	second := &salmon.Notification{}
	ch := make(chan *salmon.Notification, 1)
	ch <- first
	result := make(chan struct{})
	go func() {
		sendMessengerNotification(messengerWCtx{messenger: namedMessenger("test"), notificationsChan: ch}, second)
		close(result)
	}()

	if got := <-ch; got != first {
		t.Fatalf("first notification = %p, want %p", got, first)
	}
	select {
	case <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("blocking notification send did not complete after capacity became available")
	}
	if got := <-ch; got != second {
		t.Fatalf("second notification = %p, want %p", got, second)
	}
}
