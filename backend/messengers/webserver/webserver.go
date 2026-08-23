package webserver

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/messengers"

	"github.com/juju/errors"
	"goji.io"
	"goji.io/pat"
)

type Webserver struct {
	params Params

	server   *http.Server
	listener net.Listener

	subs      map[int]chan *salmon.Notification
	wsConns   map[int]*wsConn
	nextSubID int
	closing   bool
	subsMtx   sync.Mutex
}

var _ messengers.Messenger = &Webserver{}

type Params struct {
	Common messengers.Params

	Config Config
}

func New(params Params) (*Webserver, error) {
	if params.Config.ListenAddress == "" {
		return nil, errors.Errorf("listen address can't be empty")
	}

	s := &Webserver{
		params: params,

		subs:    make(map[int]chan *salmon.Notification),
		wsConns: make(map[int]*wsConn),
	}

	handler, err := s.createHandler()
	if err != nil {
		return nil, errors.Trace(err)
	}

	server := &http.Server{
		Addr:    params.Config.ListenAddress,
		Handler: handler,
	}

	listener, err := net.Listen("tcp", params.Config.ListenAddress)
	if err != nil {
		return nil, errors.Annotate(err, "listening")
	}

	s.server = server
	s.listener = listener

	go s.serve()
	go s.run()

	fmt.Println("Webserver is listening on", listener.Addr())

	return s, nil
}

func (s *Webserver) String() string {
	return fmt.Sprintf("webserver on %s", s.listener.Addr())
}

// Addr returns the address of the active listener.
func (s *Webserver) Addr() net.Addr { return s.listener.Addr() }

func (s *Webserver) serve() {
	_ = s.server.Serve(s.listener)

	close(s.params.Common.TornDown)
}

func (s *Webserver) run() {
	for notif := range s.params.Common.NotificationsChan {
		s.subsMtx.Lock()
		for _, sub := range s.subs {
			select {
			case sub <- notif:
				// All good
			default:
				fmt.Fprintf(os.Stderr, "failed to send notification to ws conn: buffer is full")
				// TODO: better error handling
			}

		}
		s.subsMtx.Unlock()
	}

	// Input channel was closed, so teardown now
	s.closeWebsocketConnections()
	s.server.Close()
}

func (s *Webserver) createHandler() (http.Handler, error) {
	rRoot := goji.NewMux()

	rAPI := goji.SubMux()
	rRoot.Handle(pat.New("/api/v1/*"), rAPI)
	{
		rAPI.Use(makeDesiredContentTypeMiddleware("application/json"))
		rAPI.HandleFunc(pat.Get("/status"), makeAPIHandlerWWriter(s.status))
		rAPI.HandleFunc(pat.Get("/wsconnect"), makeAPIHandlerWWriter(s.wsConnect))
	}

	return rRoot, nil
}

func (s *Webserver) status(w http.ResponseWriter, r *http.Request) (resp interface{}, err error) {
	return map[string]interface{}{
		"ongoingIncidents": s.params.Common.ItemsBoard.Get(),
	}, nil
}

func (s *Webserver) subscribe(conn *wsConn) (subID int, ch chan *salmon.Notification, ok bool) {
	s.subsMtx.Lock()
	defer s.subsMtx.Unlock()

	// Shutdown marks closing while holding this same mutex before taking its
	// snapshot of active connections. Rejecting registration here ensures a
	// connection cannot be upgraded concurrently and escape that snapshot.
	if s.closing {
		return 0, nil, false
	}

	subID = s.nextSubID
	s.nextSubID++

	ch = make(chan *salmon.Notification, 32)
	s.subs[subID] = ch
	s.wsConns[subID] = conn

	return subID, ch, true
}

// unsubscribe removes a websocket connection from notification fan-out and
// stops all work owned by that subscription. It is called both when an
// individual connection's receive loop ends and when server shutdown closes
// all remaining connections. It is safe to call more than once for the same
// ID: only the call that removes the subscription closes its channel, cancels
// its context, and closes its connection.
func (s *Webserver) unsubscribe(subID int) {
	s.subsMtx.Lock()
	ch, exists := s.subs[subID]
	conn := s.wsConns[subID]
	if exists {
		delete(s.subs, subID)
		delete(s.wsConns, subID)
		close(ch)
	}
	s.subsMtx.Unlock()
	if exists {
		conn.ctxCancel()
		_ = conn.conn.Close()
	}
}

func (s *Webserver) closeWebsocketConnections() {
	s.subsMtx.Lock()
	// Close admission before taking the snapshot. A handler that already
	// upgraded its HTTP connection will subsequently be rejected by subscribe.
	s.closing = true
	ids := make([]int, 0, len(s.wsConns))
	for id := range s.wsConns {
		ids = append(ids, id)
	}
	s.subsMtx.Unlock()
	for _, id := range ids {
		s.unsubscribe(id)
	}
}
