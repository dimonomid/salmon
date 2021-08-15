package webserver

import (
	"fmt"
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

	server *http.Server

	subs      map[int]chan *salmon.Notification
	nextSubID int
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

		subs: make(map[int]chan *salmon.Notification),
	}

	handler, err := s.createHandler()
	if err != nil {
		return nil, errors.Trace(err)
	}

	server := &http.Server{
		Addr:    params.Config.ListenAddress,
		Handler: handler,
	}

	s.server = server

	go s.serve()
	go s.run()

	return s, nil
}

func (s *Webserver) String() string {
	return fmt.Sprintf("webserver on %s", s.params.Config.ListenAddress)
}

func (s *Webserver) serve() {
	s.server.ListenAndServe()

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

func (s *Webserver) subscribe() (subID int, ch chan *salmon.Notification) {
	s.subsMtx.Lock()
	defer s.subsMtx.Unlock()

	subID = s.nextSubID
	s.nextSubID++

	ch = make(chan *salmon.Notification, 32)
	s.subs[subID] = ch

	return subID, ch
}
