package wsclient

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/logs"

	"github.com/gorilla/websocket"
)

type wsMsgServer struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

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
	Logger *logs.Logger

	OngoingIncidentsCh chan<- *salmon.Notification
	ConnErrorCh        chan<- string
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
	if params.Logger == nil {
		panic("Logger is required")
	}
	params.Logger = params.Logger.WithNamespaceAppended("WSClient")
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

		c.params.Logger.Log(logs.Info, "Connecting to %s (%s)", c.params.Config.ID, ustr)
		conn, _, err := websocket.DefaultDialer.Dial(ustr, nil)
		if err != nil {
			connError = err.Error()
			if !c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindDisconnected, Time: time.Now()}) {
				return
			}
			c.params.Logger.Log(logs.Warning, "Failed to connect to %s (%s): %s", c.params.Config.ID, ustr, err)
			continue mainLoop
		}

		c.params.Logger.Log(logs.Info, "Connected to %s (%s)", c.params.Config.ID, ustr)
		if !c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindConnected, Time: time.Now()}) {
			_ = conn.Close()
			return
		}

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
			disconnectWithError := func(err error) {
				connError = err.Error()
				c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindDisconnected, Time: time.Now()})
				c.params.Logger.Log(logs.Error, "Invalid message from %s: %s", c.params.Config.ID, err)
				_ = conn.Close()
			}
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					connError = err.Error()
					c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindDisconnected, Time: time.Now()})
					select {
					case <-c.interrupt:
						c.params.Logger.Log(logs.Debug, "Connection to %s closed", c.params.Config.ID)
					default:
						c.params.Logger.Log(logs.Warning, "Connection to %s was lost: %s", c.params.Config.ID, err)
					}
					return
				}

				// Just received something from the server: reset the read timeout. We
				// don't bother to check whether the timer has already fired: if so,
				// we'll reconnect anyway.
				readTimer.Reset(readTimeout)

				if len(message) == 1 && message[0] == 0x00 {
					// Heartbeat
					if !c.sendConnectionEvent(ConnectionEvent{EventKind: EventKindHeartbeat, Time: time.Now()}) {
						return
					}
					c.params.Logger.Log(logs.Debug, "Received heartbeat from %s", c.params.Config.ID)
					continue
				}

				var msgServer wsMsgServer
				if err := json.Unmarshal(message, &msgServer); err != nil {
					disconnectWithError(fmt.Errorf("decoding server message: %w", err))
					return
				}

				switch msgServer.Event {
				case "OngoingIncidentsSnapshot", "OngoingIncidentsUpdate":
					var notif *salmon.Notification

					if err := json.Unmarshal(msgServer.Data, &notif); err != nil {
						disconnectWithError(fmt.Errorf("decoding %s data: %w", msgServer.Event, err))
						return
					}
					if notif == nil {
						disconnectWithError(fmt.Errorf("decoding %s data: notification is null", msgServer.Event))
						return
					}
					updateKind := "update"
					if msgServer.Event == "OngoingIncidentsSnapshot" {
						updateKind = "snapshot"
					}
					total, err := json.Marshal(notif.OngoingIncidents.Total)
					if err != nil {
						c.params.Logger.Log(logs.Error, "Failed to format incident %s from %s for logging: %s",
							updateKind, c.params.Config.ID, err)
					} else {
						c.params.Logger.Log(logs.Info, "Received incident %s from %s; ongoing incidents: %s",
							updateKind, c.params.Config.ID, total)
					}

					if !c.sendOngoingIncidents(notif) {
						return
					}
				default:
					c.params.Logger.Log(logs.Warning, "Ignoring unsupported event %q from %s", msgServer.Event, c.params.Config.ID)
				}
			}
		}()

		for {
			select {

			case <-disconnected:
				readTimer.Stop()
				_ = conn.Close()
				continue mainLoop

			case <-c.interrupt:
				c.params.Logger.Log(logs.Debug, "Closing connection to %s", c.params.Config.ID)
				readTimer.Stop()
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *WSClient) sendOngoingIncidents(notif *salmon.Notification) bool {
	select {
	case c.params.OngoingIncidentsCh <- notif:
		return true
	case <-c.interrupt:
		return false
	default:
	}

	started := time.Now()
	c.params.Logger.Log(logs.Warning, "Incident delivery from %s is blocked; waiting for the consumer", c.params.Config.ID)
	select {
	case c.params.OngoingIncidentsCh <- notif:
		c.params.Logger.Log(logs.Warning, "Incident delivery from %s resumed after %s", c.params.Config.ID, time.Since(started))
		return true
	case <-c.interrupt:
		return false
	}
}

func (c *WSClient) sendConnectionEvent(event ConnectionEvent) bool {
	if c.params.ConnectionEventCh == nil {
		return true
	}
	select {
	case c.params.ConnectionEventCh <- event:
		return true
	case <-c.interrupt:
		return false
	default:
	}

	started := time.Now()
	c.params.Logger.Log(logs.Warning, "%s event delivery from %s is blocked; waiting for the consumer", event.EventKind, c.params.Config.ID)
	select {
	case c.params.ConnectionEventCh <- event:
		c.params.Logger.Log(logs.Warning, "%s event delivery from %s resumed after %s", event.EventKind, c.params.Config.ID, time.Since(started))
		return true
	case <-c.interrupt:
		return false
	}
}
