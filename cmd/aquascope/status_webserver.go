package main

import (
	"bytes"
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

// latestOngoingIncidents keeps a copy of the most recently combined incident
// snapshot for the local status UI.
type latestOngoingIncidents struct {
	mtx   sync.RWMutex
	items []salmon.ItemWContext
}

// Set replaces the snapshot with copies of the supplied items, so later
// mutations by the websocket-combiner goroutine cannot affect the status UI.
func (o *latestOngoingIncidents) Set(items []*salmon.ItemWContext) {
	o.mtx.Lock()
	defer o.mtx.Unlock()

	o.items = make([]salmon.ItemWContext, 0, len(items))
	for _, item := range items {
		if item != nil {
			o.items = append(o.items, *item)
		}
	}
}

// Get returns a copy so HTTP and websocket clients never share mutable items
// with the websocket-combiner goroutine.
func (o *latestOngoingIncidents) Get() []salmon.ItemWContext {
	o.mtx.RLock()
	defer o.mtx.RUnlock()

	items := make([]salmon.ItemWContext, len(o.items))
	copy(items, o.items)
	return items
}

// statusWebserver serves the local status UI and pushes complete incident
// snapshots to every browser connected to its websocket endpoint.
type statusWebserver struct {
	ongoingIncidents latestOngoingIncidents

	clientsMtx sync.Mutex
	clients    map[*statusWebsocketClient]struct{}
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
		Total []salmon.ItemWContext `json:"total"`
	} `json:"ongoingIncidents"`
}

func newStatusWebserver() *statusWebserver {
	return &statusWebserver{
		clients: make(map[*statusWebsocketClient]struct{}),
	}
}

// SetOngoingIncidents records a snapshot and publishes it to connected status
// pages.
func (s *statusWebserver) SetOngoingIncidents(items []*salmon.ItemWContext) {
	s.ongoingIncidents.Set(items)
	s.broadcast(s.message())
}

// message constructs a complete API snapshot rather than exposing deltas to
// the browser.
func (s *statusWebserver) message() statusWebsocketMessage {
	message := statusWebsocketMessage{}
	message.OngoingIncidents.Total = s.ongoingIncidents.Get()
	return message
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
	// Send the current snapshot while holding clientsMtx, so subsequent
	// broadcasts cannot overtake it.
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
