package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/wsclient"
)

type recordedNotification struct {
	Title string
	Text  string
}

type recordingNotificator struct {
	mu      sync.Mutex
	records []recordedNotification
}

func (n *recordingNotificator) Push(title, text string) {
	n.mu.Lock()
	n.records = append(n.records, recordedNotification{Title: title, Text: text})
	n.mu.Unlock()
}

func (n *recordingNotificator) titles() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	titles := make([]string, 0, len(n.records))
	for _, record := range n.records {
		titles = append(titles, record.Title)
	}
	return titles
}

type mockSalmonServer struct {
	server      *httptest.Server
	mu          sync.Mutex
	connections map[*websocket.Conn]struct{}
	connected   chan struct{}
}

func newMockSalmonServer(t *testing.T) *mockSalmonServer {
	t.Helper()
	mock := &mockSalmonServer{
		connections: make(map[*websocket.Conn]struct{}),
		connected:   make(chan struct{}, 16),
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mock.mu.Lock()
		mock.connections[conn] = struct{}{}
		mock.mu.Unlock()
		mock.connected <- struct{}{}
		for {
			if _, _, err := conn.NextReader(); err != nil {
				mock.mu.Lock()
				delete(mock.connections, conn)
				mock.mu.Unlock()
				_ = conn.Close()
				return
			}
		}
	})
	mock.server = newLocalTestServer(handler)
	t.Cleanup(mock.server.Close)
	return mock
}

func newLocalTestServer(handler http.Handler) *httptest.Server {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func (m *mockSalmonServer) address() string {
	return strings.TrimPrefix(m.server.URL, "http://")
}

func (m *mockSalmonServer) waitConnected(t *testing.T) {
	t.Helper()
	select {
	case <-m.connected:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for AquaScope connection")
	}
}

func (m *mockSalmonServer) send(t *testing.T, notification salmon.Notification) {
	t.Helper()
	payload, err := json.Marshal(struct {
		Event string              `json:"event"`
		Data  salmon.Notification `json:"data"`
	}{Event: "OngoingIncidentsUpdate", Data: notification})
	if err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	connections := make([]*websocket.Conn, 0, len(m.connections))
	for conn := range m.connections {
		connections = append(connections, conn)
	}
	m.mu.Unlock()
	if len(connections) == 0 {
		t.Fatal("mock Salmon has no connected AquaScope client")
	}
	for _, conn := range connections {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			t.Fatal(err)
		}
	}
}

type statusMessage struct {
	OngoingIncidents struct {
		Alerting []salmon.ItemWContext `json:"alerting"`
		Snoozed  []snoozedIncident     `json:"snoozed"`
	} `json:"ongoingIncidents"`
}

func connectStatus(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/wsconnect"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readStatus(t *testing.T, conn *websocket.Conn) statusMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var message statusMessage
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	return message
}

func item(key, comment string) *salmon.ItemWContext {
	return &salmon.ItemWContext{
		Item:       salmon.Item{Key: salmon.ItemKey(key), State: salmon.ItemStateError, Comment: comment},
		ChangeTime: time.Now(),
	}
}

func TestCoreCombinesTwoSalmonServers(t *testing.T) {
	// Model two independent Salmon hosts; AquaScope must combine their
	// incidents and prefix each key with the configured host ID.
	first := newMockSalmonServer(t)
	second := newMockSalmonServer(t)
	notifications := &recordingNotificator{}
	core, err := newAquascopeCore(aquascopeCoreParams{
		Config: wsclient.Config{Servers: []wsclient.ConfigServer{
			{ID: "first", Addr: first.address()},
			{ID: "second", Addr: second.address()},
		}},
		StatePath:     t.TempDir() + "/state.json",
		Notifications: notifications,
	})
	if err != nil {
		t.Fatal(err)
	}
	status := newLocalTestServer(rootHandler(core.statusWebserver))
	t.Cleanup(status.Close)
	statusConn := connectStatus(t, status.URL)
	// A newly connected browser receives the current, initially empty snapshot.
	_ = readStatus(t, statusConn)
	first.waitConnected(t)
	second.waitConnected(t)

	// Both hosts report a newly added error. Each should produce a status update
	// and one desktop notification, with the host prefix included in the key.
	first.send(t, salmon.Notification{OngoingIncidents: salmon.OngoingIncidentsWDelta{
		Total: []*salmon.ItemWContext{item("disk", "first")},
		Added: []*salmon.ItemWContext{item("disk", "first")},
	}})
	second.send(t, salmon.Notification{OngoingIncidents: salmon.OngoingIncidentsWDelta{
		Total: []*salmon.ItemWContext{item("cpu", "second")},
		Added: []*salmon.ItemWContext{item("cpu", "second")},
	}})

	// The two Salmon messages produce two AquaScope WebSocket updates. The
	// second one must contain the combined incidents from both hosts.
	message := readStatus(t, statusConn)
	message = readStatus(t, statusConn)
	keys := map[string]bool{}
	for _, incident := range message.OngoingIncidents.Alerting {
		keys[string(incident.Key)] = true
	}
	if !keys["first.disk"] || !keys["second.cpu"] {
		t.Fatalf("combined status missing incidents: %#v", keys)
	}
	// Notification order is intentionally not asserted because the two source
	// WebSockets are concurrent.
	gotTitles := notifications.titles()
	gotTitleSet := map[string]bool{}
	for _, title := range gotTitles {
		gotTitleSet[title] = true
	}
	if len(gotTitles) != 2 || !gotTitleSet["error: first.disk"] || !gotTitleSet["error: second.cpu"] {
		t.Fatalf("unexpected notifications: %#v", gotTitles)
	}
}

func TestCoreSnoozedIncidentDoesNotNotify(t *testing.T) {
	// Model a user snoozing an incident before it appears in a Salmon snapshot.
	// The incident should still be visible in the status UI, but must not alert.
	salmonServer := newMockSalmonServer(t)
	notifications := &recordingNotificator{}
	core, err := newAquascopeCore(aquascopeCoreParams{
		Config:        wsclient.Config{Servers: []wsclient.ConfigServer{{ID: "server", Addr: salmonServer.address()}}},
		StatePath:     t.TempDir() + "/state.json",
		Notifications: notifications,
	})
	if err != nil {
		t.Fatal(err)
	}
	status := newLocalTestServer(rootHandler(core.statusWebserver))
	t.Cleanup(status.Close)
	statusConn := connectStatus(t, status.URL)
	_ = readStatus(t, statusConn)
	salmonServer.waitConnected(t)

	// Snoozing is performed through the same HTTP API used by the real browser.
	request, err := json.Marshal(map[string]string{"key": "server.disk", "duration": "1h"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(status.URL+"/api/v1/snooze", "application/json", bytes.NewReader(request))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("snooze returned HTTP %d", response.StatusCode)
	}
	_ = response.Body.Close()
	_ = readStatus(t, statusConn)

	// Salmon now reports the incident as newly added. AquaScope should classify
	// it as snoozed in the WebSocket payload and emit no desktop notification.
	incident := item("disk", "network is down")
	salmonServer.send(t, salmon.Notification{OngoingIncidents: salmon.OngoingIncidentsWDelta{
		Total: []*salmon.ItemWContext{incident},
		Added: []*salmon.ItemWContext{incident},
	}})
	message := readStatus(t, statusConn)
	if len(message.OngoingIncidents.Snoozed) != 1 || string(message.OngoingIncidents.Snoozed[0].Key) != "server.disk" {
		t.Fatalf("unexpected snoozed status: %#v", message.OngoingIncidents)
	}
	if got := notifications.titles(); len(got) != 0 {
		t.Fatalf("snoozed incident generated notifications: %#v", got)
	}

	// Unsnoozing should immediately move the same incident back to alerting in
	// the browser payload; this action itself does not create a desktop alert.
	request, err = json.Marshal(map[string]string{"key": "server.disk"})
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.Post(status.URL+"/api/v1/unsnooze", "application/json", bytes.NewReader(request))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("unsnooze returned HTTP %d", response.StatusCode)
	}
	_ = response.Body.Close()
	message = readStatus(t, statusConn)
	if len(message.OngoingIncidents.Alerting) != 1 || string(message.OngoingIncidents.Alerting[0].Key) != "server.disk" {
		t.Fatalf("unexpected unsnoozed status: %#v", message.OngoingIncidents)
	}
}
