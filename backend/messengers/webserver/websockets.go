package webserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/interror"
)

type wsEvent string

type wsCmd string

type wsReqID int

type wsResult string

const (
	wsCmdAuthenticate = "Authenticate"
)

const (
	wsResultOK                        = "OK"
	wsResultErrBadRequest             = "Error.BadRequest"
	wsResultErrWrongCommand           = "Error.WrongCommand"
	wsResultErrAuthenticationRequired = "Error.AuthenticationRequired"
	wsResultErrOther                  = "Error.Other"
)

// wsReq is a websocket message sent by clients to the gitlab-reminder server
// So far clients only send a single message right after connection, to
// authenticate.
type wsReq struct {
	Command wsCmd       `json:"command"`
	ID      wsReqID     `json:"reqId"`
	Data    interface{} `json:"data,omitempty"`
}

// wsTxMsg is a websocket messags sent by gitlab-reminder server to the
// clients.
type wsTxMsg struct {
	Event wsEvent `json:"event"`

	// For responses to requests
	ReqID  wsReqID     `json:"reqId,omitempty"`
	Result wsResult    `json:"result,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

const (
	wsEventOngoingIncidentsSnapshot wsEvent = "OngoingIncidentsSnapshot"
	wsEventOngoingIncidentsUpdate   wsEvent = "OngoingIncidentsUpdate"

	// wsEventAuthnResult is sent to clients after they try to authenticate.
	wsEventAuthnResult wsEvent = "AuthnResult"

	wsEventResponse      wsEvent = "Response"
	wsEventInternalError wsEvent = "InternalError"
)

var upgrader = websocket.Upgrader{}

type wsConn struct {
	conn   *websocket.Conn
	txMsgs chan wsTxMsg

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
		conn:   c,
		txMsgs: make(chan wsTxMsg, 32),

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

	go s.notifHandleLoop(conn, ch, conn.txMsgs)
	go s.wsRxLoop(conn, conn.txMsgs)
	go s.wsTxLoop(conn, conn.txMsgs)

	return nil, nil
}

func (s *Webserver) notifHandleLoop(conn *wsConn, ch <-chan *salmon.Notification, txMsgs chan<- wsTxMsg) {
	initialItems := s.params.Common.ItemsBoard.Get()

	if !sendWSMessage(conn, txMsgs, wsTxMsg{
		Event: wsEventOngoingIncidentsSnapshot,
		Data: &salmon.Notification{
			Time: time.Now(),
			OngoingIncidents: salmon.OngoingIncidentsWDelta{
				Total: initialItems,
			},
		},
	}) {
		return
	}

	for notif := range ch {
		if !sendWSMessage(conn, txMsgs, wsTxMsg{
			Event: wsEventOngoingIncidentsUpdate,
			Data:  notif,
		}) {
			return
		}
	}
}

// sendWSMessage queues a message unless the connection has been canceled. It
// prevents producers from remaining blocked on a full queue after TX exits.
func sendWSMessage(conn *wsConn, txMsgs chan<- wsTxMsg, msg wsTxMsg) bool {
	select {
	case txMsgs <- msg:
		return true
	case <-conn.ctx.Done():
		return false
	}
}

func (s *Webserver) wsRxLoop(conn *wsConn, txMsgs chan<- wsTxMsg) (err error) {
	defer func() {
		fmt.Println("breaking out of rx loop:", err)

		s.unsubscribe(conn.subID)
	}()

	for {
		messageType, reader, err := conn.conn.NextReader()
		if err != nil {
			return errors.Trace(err)
		}

		if messageType != websocket.TextMessage {
			if !sendWSMessage(conn, txMsgs, wsTxMsg{
				Event:  wsEventResponse,
				Result: wsResultErrBadRequest,
			}) {
				return conn.ctx.Err()
			}
			continue
		}

		var req wsReq

		decoder := json.NewDecoder(reader)
		if err := decoder.Decode(&req); err != nil {
			if !sendWSMessage(conn, txMsgs, wsTxMsg{
				Event:  wsEventResponse,
				Result: wsResultErrBadRequest,
			}) {
				return conn.ctx.Err()
			}
			continue
		}

		resp := s.handleWSCommand(conn, req)
		if resp != nil && !sendWSMessage(conn, txMsgs, *resp) {
			return conn.ctx.Err()
		}
	}
}

func (s *Webserver) wsTxLoop(conn *wsConn, txMsgs <-chan wsTxMsg) {
	defer func() {
		fmt.Println("breaking out of tx loop")
		s.unsubscribe(conn.subID)
	}()

	// Create ticker for heartbeats
	heartbeats := time.NewTicker(10 * time.Second)
	defer heartbeats.Stop()

	for {
		select {
		case <-conn.ctx.Done():
			return

		case msg := <-txMsgs:
			w, err := conn.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				fmt.Println("NextWriter err:", err)
				return
			}

			encoder := json.NewEncoder(w)
			err = encoder.Encode(msg)
			if err != nil {
				fmt.Println("Encode err:", err)
				err = makeInternalServerError(err)

				errResp := getRespFromError(err)
				err2 := encoder.Encode(errResp)
				if err2 != nil {
					fmt.Println("Encode err2:", err2)
					continue
				}
			}
			if err := w.Close(); err != nil {
				fmt.Println("Close err:", err)
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

// handleWSCommand takes a request message (received from clients) and returns
// the response to be sent back.
func (s *Webserver) handleWSCommand(conn *wsConn, req wsReq) (resp *wsTxMsg) {
	defer func() {
		if resp != nil {
			resp.ReqID = req.ID
		} else {
			fmt.Println("handleWSCommand nil resp!")
		}
	}()

	//dataJSON, err := json.Marshal(req.Data)
	//if err != nil {
	//panic(err.Error())
	//}

	switch req.Command {
	case wsCmdAuthenticate:
		// TODO
		//reqAuth := wsReqAuthenticate{}
		//if err := json.Unmarshal(dataJSON, &reqAuth); err != nil {
		//return getRespFromError(err)
		//}

		//resp = s.handleWSCommandAuthenticate(conn, reqAuth)

	default:
		resp = &wsTxMsg{
			Result: wsResultErrWrongCommand,
		}
	}

	return resp
}

func getRespFromError(errGot error) *wsTxMsg {
	if interror.IsInternalError(errGot) {
		fmt.Println("INTERNAL SERVER ERROR")
		fmt.Println(interror.ErrorStack(errGot))
	}

	return &wsTxMsg{
		Event:  wsEventInternalError,
		Result: wsResultErrOther,
		Data:   errGot.Error(),
	}
}
