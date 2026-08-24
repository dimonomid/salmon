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
	"github.com/dimonomid/salmon/backend/messengers"
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

func TestWebsocketSubscriptionQueueSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection := &wsConn{ctx: ctx, ctxCancel: cancel}
	webserver := &Webserver{
		subs:    make(map[int]chan *salmon.Notification),
		wsConns: make(map[int]*wsConn),
	}

	_, notifications, ok := webserver.subscribe(connection)
	if !ok {
		t.Fatal("subscription was rejected")
	}
	if got := cap(notifications); got != 64 {
		t.Fatalf("subscription queue capacity = %d, want 64", got)
	}
}

func TestRunDisconnectsSubscriberWhoseQueueIsFull(t *testing.T) {
	incoming := make(chan *salmon.Notification, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection := &wsConn{ctx: ctx, ctxCancel: cancel}
	webserver := &Webserver{
		params:  Params{Common: messengers.Params{NotificationsChan: incoming}},
		server:  &http.Server{},
		subs:    make(map[int]chan *salmon.Notification),
		wsConns: make(map[int]*wsConn),
	}
	id, notifications, ok := webserver.subscribe(connection)
	if !ok {
		t.Fatal("subscription was rejected")
	}
	for i := 0; i < cap(notifications); i++ {
		notifications <- &salmon.Notification{}
	}
	runDone := make(chan struct{})
	go func() {
		webserver.run()
		close(runDone)
	}()
	incoming <- &salmon.Notification{}

	select {
	case <-connection.ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("subscriber with a full notification queue was not canceled")
	}
	webserver.subsMtx.Lock()
	_, subscribed := webserver.subs[id]
	webserver.subsMtx.Unlock()
	if subscribed {
		t.Fatal("subscriber with a full notification queue remains registered")
	}

	close(incoming)
	select {
	case <-runDone:
	case <-time.After(3 * time.Second):
		t.Fatal("webserver run loop did not stop")
	}
}
