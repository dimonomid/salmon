package wsclient_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/wsclient"
	"github.com/gorilla/websocket"
)

func TestClientDisconnectsFromMalformedServerMessages(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "malformed envelope", message: `{"event":`},
		{name: "malformed notification", message: `{"event":"OngoingIncidentsUpdate","data":"invalid"}`},
		{name: "null notification", message: `{"event":"OngoingIncidentsSnapshot","data":null}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverConnections := make(chan *websocket.Conn, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				serverConnections <- connection
				for {
					if _, _, err := connection.NextReader(); err != nil {
						_ = connection.Close()
						return
					}
				}
			}))
			t.Cleanup(server.Close)

			notifications := make(chan *salmon.Notification, 1)
			connectionErrors := make(chan string, 8)
			connectionEvents := make(chan wsclient.ConnectionEvent, 8)
			client, err := wsclient.New(wsclient.Params{
				Config:             wsclient.ConfigServer{ID: "test", Addr: strings.TrimPrefix(server.URL, "http://")},
				OngoingIncidentsCh: notifications,
				ConnErrorCh:        connectionErrors,
				ReconnectDelay:     time.Hour,
				ConnectionEventCh:  connectionEvents,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)

			var connection *websocket.Conn
			select {
			case connection = <-serverConnections:
			case <-time.After(3 * time.Second):
				t.Fatal("client did not connect to test server")
			}
			if err := connection.WriteMessage(websocket.TextMessage, []byte(test.message)); err != nil {
				t.Fatal(err)
			}

			deadline := time.After(3 * time.Second)
			for {
				select {
				case event := <-connectionEvents:
					if event.EventKind == wsclient.EventKindDisconnected {
						reported := false
						for !reported {
							select {
							case connectionError := <-connectionErrors:
								if connectionError != "" {
									reported = true
								}
							case <-time.After(3 * time.Second):
								t.Fatal("malformed message was not reported as a connection error")
							}
						}
						select {
						case notification := <-notifications:
							t.Fatalf("malformed message produced notification %#v", notification)
						default:
						}
						return
					}
				case <-deadline:
					t.Fatal("client did not disconnect from malformed server message")
				}
			}
		})
	}
}
