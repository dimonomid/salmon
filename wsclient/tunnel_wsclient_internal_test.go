package wsclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/gorilla/websocket"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/logs"
)

func TestWSClientWaitsForTunnelReadiness(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		for {
			if _, _, err := connection.NextReader(); err != nil {
				return
			}
		}
	}))

	logger := logs.NewLogger(logs.LoggerParams{Clock: clock.New()})
	tunnel := NewTunnelSupervisor(TunnelSupervisorParams{
		ServerID: "remote",
		Command: TunnelCommandSpec{
			Command:              []string{"sh", "-c", "sleep 0.15; echo tunnel-ready; exec sleep 30"},
			ReadinessProbeString: "tunnel-ready",
		},
		Logger:       logger,
		RestartDelay: time.Hour,
	})
	connectionEvents := make(chan ConnectionEvent, 8)
	client, err := New(Params{
		Config:             ConfigServer{ID: "remote", Addr: strings.TrimPrefix(server.URL, "http://")},
		Logger:             logger,
		OngoingIncidentsCh: make(chan *salmon.Notification, 8),
		ConnErrorCh:        make(chan string, 8),
		ReconnectDelay:     time.Hour,
		ConnectionEventCh:  connectionEvents,
		Tunnel:             tunnel,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close()
		tunnel.Close()
		server.Close()
	})

	select {
	case <-requests:
		t.Fatal("WebSocket connection was attempted before tunnel readiness")
	case event := <-connectionEvents:
		t.Fatalf("connection event %#v was published before tunnel readiness", event)
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("WebSocket connection was not attempted after tunnel readiness")
	}
}
