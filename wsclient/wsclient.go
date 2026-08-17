package wsclient

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/dimonomid/salmon"

	"github.com/gorilla/websocket"
)

type wsReq struct {
	Command wsCmd       `json:"command"`
	ID      wsReqID     `json:"reqId"`
	Data    interface{} `json:"data,omitempty"`
}

type wsMsgServer struct {
	Event  string          `json:"event"`
	Result string          `json:"result,omitempty"`
	ID     wsReqID         `json:"reqId,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type wsReqAuthenticate struct {
	// TODO
}

type wsCmd string
type wsReqID int

const (
	wsCmdAuthenticate = "Authenticate"
)

const (
	// heartbeatPeriod is how often the server is expected to send heartbeats
	heartbeatPeriod = 10 * time.Second

	// readTimeout specifies how long to wait for any data received from the
	// server before disconnecting.
	readTimeout = heartbeatPeriod * 3
)

type WSClient struct {
	params Params
	// interrupt is closed to stop dialing, reconnect delays, and the active
	// connection.
	interrupt chan struct{}
	// closeOnce makes Close safe to call from multiple cleanup paths.
	closeOnce sync.Once
}

type Params struct {
	Config ConfigServer

	OngoingIncidentsCh    chan<- *salmon.Notification
	ConnErrorCh           chan<- string
	ServerInternalErrorCh chan<- string
	// ReconnectDelay overrides the production reconnect delay when non-zero.
	ReconnectDelay time.Duration
	// ConnectionEventCh receives connection and heartbeat events.
	ConnectionEventCh chan<- ConnectionEvent
}

// ConnectionEventKind identifies the kind of connection event.
type ConnectionEventKind string

const (
	EventKindConnected    ConnectionEventKind = "connected"
	EventKindDisconnected ConnectionEventKind = "disconnected"
	EventKindHeartbeat    ConnectionEventKind = "heartbeat"
)

// ConnectionEvent describes a Salmon connection transition or heartbeat.
type ConnectionEvent struct {
	EventKind ConnectionEventKind
	Time      time.Time
}

func New(params Params) (*WSClient, error) {
	c := &WSClient{
		params:    params,
		interrupt: make(chan struct{}),
	}

	go c.eventLoop()

	return c, nil
}

// Close stops the client and returns immediately; the owning Combiner waits
// for the client worker to finish.
func (c *WSClient) Close() {
	c.closeOnce.Do(func() { close(c.interrupt) })
}

func (c *WSClient) eventLoop() {
	connError := ""

mainLoop:
	for i := 0; true; i++ {
		select {
		case c.params.ConnErrorCh <- connError:
		case <-c.interrupt:
			return
		}

		if i > 0 {
			delay := 5 * time.Second
			if c.params.ReconnectDelay > 0 {
				delay = c.params.ReconnectDelay
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-c.interrupt:
				if !timer.Stop() {
					<-timer.C
				}
				return
			}
		}

		u := url.URL{
			Scheme: "ws",
			Host:   c.params.Config.Addr,
			Path:   "/api/v1/wsconnect",
		}

		ustr := u.String()

		fmt.Println("Connecting to", ustr)
		conn, _, err := websocket.DefaultDialer.Dial(ustr, nil)
		if err != nil {
			connError = err.Error()
			c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindDisconnected, Time: time.Now()})
			fmt.Println("Connection error:", err)
			continue mainLoop
		}

		fmt.Println("Connected")
		c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindConnected, Time: time.Now()})

		/*

					authnMsg := wsReqAuthenticate{
			      // TODO
					}

					authnReq := wsReq{
						Command: wsCmdAuthenticate,
						Data:    authnMsg,
					}

					data, err := json.Marshal(authnReq)
					if err != nil {
						fmt.Println("Error marshaling authn msg:", err)
						os.Exit(1)
					}

					if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
						connError = err.Error()
						fmt.Println("Write error:", err)
						continue mainLoop
					}

		*/

		connError = ""
		select {
		case c.params.ConnErrorCh <- connError:
		case <-c.interrupt:
			_ = conn.Close()
			return
		}

		disconnected := make(chan struct{})

		readTimer := time.AfterFunc(readTimeout, func() {
			// We haven't heard anything from the server for too long, so,
			// disconnect.
			//
			// NOTE that we don't use the c.Close() method: it's quite likely that
			// there is some problem with the network, and c.Close() first tries to
			// send the websocket close message, which would take quite some time
			// before timing out. Instead we just close the ws connection
			// forcefully, thus immediately breaking out of recvLoop,
			// so the clients will notice the state change right away.
			conn.Close()
		})

		go func() {
			defer func() {
				close(disconnected)
			}()
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					connError = err.Error()
					c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindDisconnected, Time: time.Now()})
					fmt.Println("Read error:", err)
					return
				}

				// Just received something from the server: reset the read timeout. We
				// don't bother to check whether the timer has already fired: if so,
				// we'll reconnect anyway.
				readTimer.Reset(readTimeout)

				if len(message) == 1 && message[0] == 0x00 {
					// Heartbeat
					c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindHeartbeat, Time: time.Now()})
					fmt.Println(c.params.Config.ID, "heartbeat")
					continue
				}

				fmt.Println("Recv:", string(message))

				var msgServer wsMsgServer
				if err := json.Unmarshal(message, &msgServer); err != nil {
					fmt.Println("Decode error:", err)
				}

				switch msgServer.Event {
				case "OngoingIncidentsSnapshot", "OngoingIncidentsUpdate":
					var notif *salmon.Notification

					if err := json.Unmarshal(msgServer.Data, &notif); err != nil {
						fmt.Println("InternalError decode error:", err)
					}

					select {
					case c.params.OngoingIncidentsCh <- notif:
						// All good
					default:
						// TODO: better error handling
						fmt.Println("failed to send ongoing incidents update: buffer full")
					}

				case "AuthnResult":
					fmt.Println("TODO AuthnResult")
					//if msgServer.Result == "OK" {
					//var authnData webserver.AuthnMsg

					//if err := json.Unmarshal(dataJSON, &authnData); err != nil {
					//fmt.Println("authn msg decode error:", err)
					//continue
					//}

					//if c.params.AuthnHandler != nil {
					//c.params.AuthnHandler(authnData.User, "")
					//}
					//} else {
					//if c.params.AuthnHandler != nil {
					//c.params.AuthnHandler(nil, string(dataJSON))
					//}
					//}

				case "InternalError":
					var errStr string

					if err := json.Unmarshal(msgServer.Data, &errStr); err != nil {
						fmt.Println("InternalError decode error:", err)
					}

					select {
					case c.params.ServerInternalErrorCh <- errStr:
						// All good
					default:
						// TODO: better error handling
						fmt.Println("failed to send internal server error: buffer full")
					}
				}
			}
		}()

		for {
			select {

			case <-disconnected:
				continue mainLoop

			case <-c.interrupt:
				fmt.Println("Closing connection")
				readTimer.Stop()
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *WSClient) sendConnectionEvent(event ConnectionEvent) {
	if c.params.ConnectionEventCh == nil {
		return
	}
	select {
	case c.params.ConnectionEventCh <- event:
	default:
		fmt.Println("failed to send connection event: buffer full")
	}
}
