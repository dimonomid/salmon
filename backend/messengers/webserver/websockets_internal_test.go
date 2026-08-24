package webserver

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/gorilla/websocket"
)

func TestWSTransmitFailureUnsubscribesConnection(t *testing.T) {
	serverConnections := make(chan *websocket.Conn, 1)
	handlerDone := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnections <- connection
		<-handlerDone
	}))
	t.Cleanup(func() {
		close(handlerDone)
		httpServer.Close()
	})

	clientConnection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	serverConnection := <-serverConnections

	ctx, cancel := context.WithCancel(context.Background())
	connection := &wsConn{
		conn:      serverConnection,
		txMsgs:    make(chan wsTxMsg, 128),
		ctx:       ctx,
		ctxCancel: cancel,
	}
	webserver := &Webserver{
		subs:    make(map[int]chan *salmon.Notification),
		wsConns: make(map[int]*wsConn),
	}
	connection.subID, _, _ = webserver.subscribe(connection)

	// Resetting the client TCP connection makes the server's write side fail
	// without a server receive loop performing cleanup first.
	tcpConnection, ok := clientConnection.UnderlyingConn().(*net.TCPConn)
	if !ok {
		t.Fatalf("underlying connection has type %T, want *net.TCPConn", clientConnection.UnderlyingConn())
	}
	if err := tcpConnection.SetLinger(0); err != nil {
		t.Fatal(err)
	}
	if err := clientConnection.Close(); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < cap(connection.txMsgs); i++ {
		connection.txMsgs <- wsTxMsg{Event: wsEventOngoingIncidentsUpdate}
	}
	go webserver.wsTxLoop(connection, connection.txMsgs)

	select {
	case <-connection.ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("transmit failure did not unsubscribe the connection")
	}

	webserver.subsMtx.Lock()
	_, subscribed := webserver.subs[connection.subID]
	webserver.subsMtx.Unlock()
	if subscribed {
		t.Fatal("connection remains registered after transmit failure")
	}
}

func TestSendWSMessageUnblocksWhenConnectionIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	connection := &wsConn{ctx: ctx, ctxCancel: cancel}
	messages := make(chan wsTxMsg, 1)
	messages <- wsTxMsg{Event: wsEventOngoingIncidentsUpdate}

	result := make(chan bool, 1)
	go func() {
		result <- sendWSMessage(connection, messages, wsTxMsg{Event: wsEventOngoingIncidentsUpdate})
	}()

	// The queue is full, so cancellation must release the blocked producer.
	cancel()
	select {
	case sent := <-result:
		if sent {
			t.Fatal("sendWSMessage reported sending after cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sendWSMessage remained blocked after cancellation")
	}
}

func TestSendNotificationToSubscriberWaitsForCapacity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection := &wsConn{ctx: ctx, ctxCancel: cancel}
	first := &salmon.Notification{}
	second := &salmon.Notification{}
	notifications := make(chan *salmon.Notification, 1)
	notifications <- first
	result := make(chan bool, 1)
	go func() {
		result <- sendNotificationToSubscriber(7, connection, notifications, second)
	}()

	if got := <-notifications; got != first {
		t.Fatalf("first notification = %p, want %p", got, first)
	}
	select {
	case sent := <-result:
		if !sent {
			t.Fatal("notification send was canceled")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocking subscriber send did not complete after capacity became available")
	}
	if got := <-notifications; got != second {
		t.Fatalf("second notification = %p, want %p", got, second)
	}
}

func TestSendNotificationToSubscriberUnblocksWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	connection := &wsConn{ctx: ctx, ctxCancel: cancel}
	notifications := make(chan *salmon.Notification, 1)
	notifications <- &salmon.Notification{}
	result := make(chan bool, 1)
	go func() {
		result <- sendNotificationToSubscriber(7, connection, notifications, &salmon.Notification{})
	}()

	cancel()
	select {
	case sent := <-result:
		if sent {
			t.Fatal("notification was reported sent after cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocking subscriber send did not unblock after cancellation")
	}
}
