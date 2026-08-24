package webserver_test

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/itemsboard"
	"github.com/dimonomid/salmon/backend/messengers"
	server "github.com/dimonomid/salmon/backend/messengers/webserver"
)

type websocketEnvelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

func TestServerPublishesStatusSnapshotAndUpdates(t *testing.T) {
	board := itemsboard.New()
	initial := incident("disk.free", salmon.ItemStateError, "full")
	board.Set([]*salmon.ItemWContext{initial})
	webserver, notifications, done := startServer(t, board)

	response, err := http.Get("http://" + webserver.Addr().String() + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status response = %s", response.Status)
	}
	var status struct {
		OngoingIncidents []*salmon.ItemWContext `json:"ongoingIncidents"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	assertIncidentKeys(t, status.OngoingIncidents, "disk.free")

	connection := dialServer(t, webserver, nil)
	initialMessage := readEnvelope(t, connection)
	if initialMessage.Event != "OngoingIncidentsSnapshot" {
		t.Fatalf("initial event = %q", initialMessage.Event)
	}
	assertNotificationTotal(t, initialMessage.Data, "disk.free")

	update := &salmon.Notification{
		Time: time.Now(),
		OngoingIncidents: salmon.OngoingIncidentsWDelta{
			Total: []*salmon.ItemWContext{incident("systemd.sync", salmon.ItemStateWarning, "failed")},
		},
	}
	notifications <- update
	updateMessage := readEnvelope(t, connection)
	if updateMessage.Event != "OngoingIncidentsUpdate" {
		t.Fatalf("update event = %q", updateMessage.Event)
	}
	assertNotificationTotal(t, updateMessage.Data, "systemd.sync")

	_ = connection.Close()
	close(notifications)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServerRejectsCrossOriginWebsockets(t *testing.T) {
	webserver, notifications, done := startServer(t, itemsboard.New())
	header := http.Header{"Origin": []string{"https://example.invalid"}}
	url := "ws://" + webserver.Addr().String() + "/api/v1/wsconnect"
	connection, response, err := websocket.DefaultDialer.Dial(url, header)
	if connection != nil {
		_ = connection.Close()
	}
	if err == nil {
		t.Fatal("cross-origin websocket was accepted")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("response = %#v, want HTTP 403", response)
	}
	close(notifications)
	<-done
}

func TestServerClosesWebsocketWhenClientSendsMessage(t *testing.T) {
	webserver, notifications, done := startServer(t, itemsboard.New())
	for name, messageType := range map[string]int{
		"text":   websocket.TextMessage,
		"binary": websocket.BinaryMessage,
	} {
		t.Run(name, func(t *testing.T) {
			connection := dialServer(t, webserver, nil)
			_ = readEnvelope(t, connection)

			if err := connection.WriteMessage(messageType, []byte(`{"command":"Authenticate"}`)); err != nil {
				t.Fatal(err)
			}
			_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, _, err := connection.ReadMessage(); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
				t.Fatalf("read error = %v, want policy-violation close", err)
			}
		})
	}

	close(notifications)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServerReportsBindFailure(t *testing.T) {
	first, firstNotifications, firstDone := startServer(t, itemsboard.New())
	notifications := make(chan *salmon.Notification)
	done := make(chan struct{})
	_, err := server.New(server.Params{
		Common: messengers.Params{ItemsBoard: itemsboard.New(), NotificationsChan: notifications, TornDown: done},
		Config: server.Config{ListenAddress: first.Addr().String()},
	})
	if err == nil {
		t.Fatal("second server on occupied address started successfully")
	}
	close(firstNotifications)
	<-firstDone
}

func TestServerShutdownClosesActiveWebsocket(t *testing.T) {
	webserver, notifications, done := startServer(t, itemsboard.New())
	connection := dialServer(t, webserver, nil)
	_ = readEnvelope(t, connection)

	close(notifications)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("websocket remained open after server shutdown")
	}
}

func TestServerShutdownRejectsConcurrentWebsocketSubscriptions(t *testing.T) {
	webserver, notifications, done := startServer(t, itemsboard.New())
	url := "ws://" + webserver.Addr().String() + "/api/v1/wsconnect"
	start := make(chan struct{})
	connections := make(chan *websocket.Conn, 64)
	var attempts sync.WaitGroup
	for i := 0; i < cap(connections); i++ {
		attempts.Add(1)
		go func() {
			defer attempts.Done()
			<-start
			connection, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err == nil {
				connections <- connection
			}
		}()
	}
	close(start)
	close(notifications)
	attempts.Wait()
	close(connections)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
	for connection := range connections {
		_ = connection.SetReadDeadline(time.Now().Add(time.Second))
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				if netError, ok := err.(net.Error); ok && netError.Timeout() {
					_ = connection.Close()
					t.Fatal("concurrently upgraded websocket survived server shutdown")
				}
				break
			}
		}
		_ = connection.Close()
	}
}

func startServer(t *testing.T, board *itemsboard.ItemsBoard) (*server.Webserver, chan *salmon.Notification, chan struct{}) {
	t.Helper()
	notifications := make(chan *salmon.Notification, 4)
	done := make(chan struct{})
	webserver, err := server.New(server.Params{
		Common: messengers.Params{ItemsBoard: board, NotificationsChan: notifications, TornDown: done},
		Config: server.Config{ListenAddress: "127.0.0.1:0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return webserver, notifications, done
}

func dialServer(t *testing.T, webserver *server.Webserver, header http.Header) *websocket.Conn {
	t.Helper()
	url := "ws://" + webserver.Addr().String() + "/api/v1/wsconnect"
	connection, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func readEnvelope(t *testing.T, connection *websocket.Conn) websocketEnvelope {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	var message websocketEnvelope
	if err := connection.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	return message
}

func assertNotificationTotal(t *testing.T, data json.RawMessage, keys ...string) {
	t.Helper()
	var notification salmon.Notification
	if err := json.Unmarshal(data, &notification); err != nil {
		t.Fatal(err)
	}
	assertIncidentKeys(t, notification.OngoingIncidents.Total, keys...)
}

func assertIncidentKeys(t *testing.T, incidents []*salmon.ItemWContext, keys ...string) {
	t.Helper()
	got := make([]string, 0, len(incidents))
	for _, value := range incidents {
		got = append(got, string(value.Key))
	}
	if strings.Join(got, ",") != strings.Join(keys, ",") {
		t.Fatalf("incident keys = %#v, want %#v", got, keys)
	}
}

func incident(key salmon.ItemKey, state salmon.ItemState, details string) *salmon.ItemWContext {
	return &salmon.ItemWContext{Item: salmon.Item{Key: key, State: state, Details: details}, IncidentStartedAt: time.Now()}
}
