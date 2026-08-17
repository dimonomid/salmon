package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/websocket"

	"github.com/dimonomid/salmon"
)

const defPort = 41991

// statusWebserver transports already-classified incident snapshots to the
// local status UI. Classification itself belongs to incidentState.
type statusWebserver struct {
	snapshotMtx sync.RWMutex
	snapshot    incidentSnapshot
	onSnooze    func(string, time.Duration) error
	onUnsnooze  func(string) error

	clientsMtx sync.Mutex
	clients    map[*statusWebsocketClient]struct{}
}

// statusWebserverParams contains the application callbacks needed by the
// status webserver.
type statusWebserverParams struct {
	// OnSnooze persists a snooze. The incident-state update hook publishes the
	// resulting snapshot to connected status pages.
	OnSnooze func(key string, duration time.Duration) error
	// OnUnsnooze removes a snooze. The incident-state update hook publishes the
	// resulting snapshot to connected status pages.
	OnUnsnooze func(key string) error
}

// statusWebsocketClient has a single websocket writer and a buffered queue of
// complete incident snapshots.
type statusWebsocketClient struct {
	conn     *websocket.Conn
	updates  chan statusWebsocketMessage
	done     chan struct{}
	doneOnce sync.Once
}

// statusWebsocketMessage is the local browser API payload.
type statusWebsocketMessage struct {
	OngoingIncidents struct {
		Alerting []salmon.ItemWContext `json:"alerting"`
		Snoozed  []snoozedIncident     `json:"snoozed"`
	} `json:"ongoingIncidents"`
}

func newStatusWebserver(params statusWebserverParams) *statusWebserver {
	return &statusWebserver{
		onSnooze:   params.OnSnooze,
		onUnsnooze: params.OnUnsnooze,
		clients:    make(map[*statusWebsocketClient]struct{}),
	}
}

// SetOngoingIncidents stores and publishes a snapshot already classified by
// incidentState.
func (s *statusWebserver) SetOngoingIncidents(snapshot incidentSnapshot) {
	s.snapshotMtx.Lock()
	s.snapshot = snapshot
	s.snapshotMtx.Unlock()
	s.broadcast(s.message())
}

// message constructs the browser API payload from the latest classified
// snapshot.
func (s *statusWebserver) message() statusWebsocketMessage {
	s.snapshotMtx.RLock()
	defer s.snapshotMtx.RUnlock()

	message := statusWebsocketMessage{}
	message.OngoingIncidents.Alerting = append([]salmon.ItemWContext(nil), s.snapshot.Alerting...)
	message.OngoingIncidents.Snoozed = append([]snoozedIncident(nil), s.snapshot.Snoozed...)
	return message
}

// snoozeRequest is the JSON body accepted by the snooze endpoint.
type snoozeRequest struct {
	Key      string `json:"key"`
	Duration string `json:"duration"`
}

type unsnoozeRequest struct {
	Key string `json:"key"`
}

func (s *statusWebserver) snooze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var request snoozeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid snooze request", http.StatusBadRequest)
		return
	}
	duration, ok := snoozeDurations[request.Duration]
	if request.Key == "" || !ok {
		http.Error(w, "invalid snooze key or duration", http.StatusBadRequest)
		return
	}

	if err := s.onSnooze(request.Key, duration); err != nil {
		http.Error(w, "failed to persist snooze", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *statusWebserver) unsnooze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var request unsnoozeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Key == "" {
		http.Error(w, "invalid unsnooze request", http.StatusBadRequest)
		return
	}

	if err := s.onUnsnooze(request.Key); err != nil {
		http.Error(w, "failed to persist unsnooze", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// broadcast sends each snapshot to every connected client, in a blocking way
// (although if we have to block, we also log a message).
func (s *statusWebserver) broadcast(message statusWebsocketMessage) {
	s.clientsMtx.Lock()
	clients := make([]*statusWebsocketClient, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMtx.Unlock()

	for _, client := range clients {
		select {
		case client.updates <- message:
		case <-client.done:
			continue
		default:
			fmt.Println("status websocket update queue is full; waiting for client")
			select {
			case client.updates <- message:
			case <-client.done:
			}
		}
	}
}

// wsConnect upgrades a local status-page connection and streams the initial
// snapshot followed by updates. Incoming messages are ignored; reading them
// only detects browser disconnects.
func (s *statusWebserver) wsConnect(w http.ResponseWriter, r *http.Request) {
	conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading status websocket:", err)
		return
	}

	client := &statusWebsocketClient{
		conn:    conn,
		updates: make(chan statusWebsocketMessage, 32),
		done:    make(chan struct{}),
	}

	s.clientsMtx.Lock()
	s.clients[client] = struct{}{}
	client.updates <- s.message()
	s.clientsMtx.Unlock()

	defer func() {
		s.clientsMtx.Lock()
		delete(s.clients, client)
		s.clientsMtx.Unlock()
		client.doneOnce.Do(func() { close(client.done) })
		conn.Close()
	}()

	go func() {
		for {
			select {
			case message := <-client.updates:
				if err := conn.WriteJSON(message); err != nil {
					conn.Close()
					return
				}
			case <-client.done:
				return
			}
		}
	}()

	for {
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}

// setupWebserver tries to create a listener on the default port (defPort);
// if that fails for whatever reason, tries to listen on a random port, and if
// that fails as well, panics.
func setupWebserver(statusWebserver *statusWebserver) net.Listener {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", defPort))
	if err != nil {
		fmt.Printf("Failed to listen on a default port %d (%s), listening on random port\n", defPort, err)
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			panic(err.Error())
		}
	}

	// The status page and browser websocket share this local HTTP server.
	http.HandleFunc("/status", serveStatusPage)
	http.HandleFunc("/api/v1/wsconnect", statusWebserver.wsConnect)
	http.HandleFunc("/api/v1/snooze", statusWebserver.snooze)
	http.HandleFunc("/api/v1/unsnooze", statusWebserver.unsnooze)
	// Reuse the same image assets as the systray icon in the status UI.
	for _, iconName := range []string{"gray", "green", "magenta", "yellow", "red"} {
		iconAssetPath := fmt.Sprintf("assets/salmon_%s.png", iconName)
		http.HandleFunc("/icons/salmon_"+iconName+".png", func(w http.ResponseWriter, r *http.Request) {
			serveIcon(w, r, iconAssetPath)
		})
	}

	http.Handle("/", noStoreHandler(webrootFileServer()))
	return listener
}

func serveStatusPage(w http.ResponseWriter, r *http.Request) {
	data, err := Asset("assets/webroot/index.html")
	if err != nil {
		http.Error(w, "status page is unavailable", http.StatusInternalServerError)
		return
	}

	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Unix(1, 0), bytes.NewReader(data))
}

// serveIcon exposes an embedded systray icon to the local status UI.
func serveIcon(w http.ResponseWriter, r *http.Request, assetName string) {
	data, err := Asset(assetName)
	if err != nil {
		http.Error(w, "icon is unavailable", http.StatusInternalServerError)
		return
	}

	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", "image/png")
	http.ServeContent(w, r, assetName, time.Unix(1, 0), bytes.NewReader(data))
}

// noStoreHandler ensures the browser does not keep stale copies of static UI
// assets after AquaScope is rebuilt with new embedded assets.
func noStoreHandler(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setNoStoreHeaders(w)
		inner.ServeHTTP(w, r)
	})
}

// setNoStoreHeaders disables caching for the status document and its assets.
func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func webrootFileServer() http.Handler {
	assetInfo := func(path string) (os.FileInfo, error) {
		return os.Stat(path)
	}

	return http.FileServer(
		&assetfs.AssetFS{
			Asset:     Asset,
			AssetDir:  AssetDir,
			AssetInfo: assetInfo,
			Prefix:    "assets/webroot",
		},
	)
}
