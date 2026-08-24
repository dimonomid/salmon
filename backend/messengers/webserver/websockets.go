package webserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"

	"github.com/dimonomid/salmon"
)

type wsEvent string

// wsTxMsg is a WebSocket event sent by the server to clients.
type wsTxMsg struct {
	Event wsEvent     `json:"event"`
	Data  interface{} `json:"data,omitempty"`
}

const (
	wsEventOngoingIncidentsSnapshot wsEvent = "OngoingIncidentsSnapshot"
	wsEventOngoingIncidentsUpdate   wsEvent = "OngoingIncidentsUpdate"
)

var upgrader = websocket.Upgrader{}

type wsConn struct {
	conn *websocket.Conn

	// ctx is a context which is canceled when the client disconnects.
	ctx       context.Context
	ctxCancel context.CancelFunc

	// subID is the subscription ID which should be used to unsubscribe from
	// updates when the connection is disconnected (unless it's 0).
	subID int
}

func (s *Webserver) wsConnect(w http.ResponseWriter, r *http.Request) (resp interface{}, err error) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	conn := &wsConn{
		conn: c,

		ctx:       ctx,
		ctxCancel: ctxCancel,
	}

	subID, ch, ok := s.subscribe(conn)
	if !ok {
		conn.ctxCancel()
		_ = conn.conn.Close()
		return nil, nil
	}
	conn.subID = subID

	go s.wsRxLoop(conn)
	go s.wsTxLoop(conn, ch)

	return nil, nil
}

func (s *Webserver) wsRxLoop(conn *wsConn) (err error) {
	defer func() {
		fmt.Println("breaking out of rx loop:", err)

		s.unsubscribe(conn.subID)
	}()
	// This is a server-to-client-only protocol. Receiving any client data
	// message is a protocol violation, so disconnect the client immediately.
	for {
		_, _, err := conn.conn.NextReader()
		if err != nil {
			return errors.Trace(err)
		}

		message := websocket.FormatCloseMessage(
			websocket.ClosePolicyViolation,
			"unsupported data",
		)
		_ = conn.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))
		return errors.New("client sent an unsupported WebSocket message")
	}
}

func (s *Webserver) wsTxLoop(conn *wsConn, notifications <-chan *salmon.Notification) {
	defer func() {
		fmt.Println("breaking out of tx loop")
		s.unsubscribe(conn.subID)
	}()

	initialItems := s.params.Common.ItemsBoard.Get()
	if err := conn.conn.WriteJSON(wsTxMsg{
		Event: wsEventOngoingIncidentsSnapshot,
		Data: &salmon.Notification{
			Time: time.Now(),
			OngoingIncidents: salmon.OngoingIncidentsWDelta{
				Total: initialItems,
			},
		},
	}); err != nil {
		fmt.Println("initial snapshot write error:", err)
		return
	}

	// Create ticker for heartbeats
	heartbeats := time.NewTicker(10 * time.Second)
	defer heartbeats.Stop()

	for {
		select {
		case <-conn.ctx.Done():
			return

		case notif := <-notifications:
			if err := conn.conn.WriteJSON(wsTxMsg{
				Event: wsEventOngoingIncidentsUpdate,
				Data:  notif,
			}); err != nil {
				fmt.Println("notification write error:", err)
				return
			}

		case <-heartbeats.C:
			if err := conn.conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
				fmt.Println("heartbeat write error:", err)
				return
			}
		}
	}
}
