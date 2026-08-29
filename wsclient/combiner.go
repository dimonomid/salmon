package wsclient

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/logs"
	"github.com/dimonomid/salmon/statestracker"
	"github.com/juju/errors"

	"github.com/benbjohnson/clock"
)

const (
	IDInternal = "internal"
)

type Combiner struct {
	params CombinerParams

	internalTracker *statestracker.ItemStatesTracker
	// internalTrackerMtx serializes FeedItems, whose tracker maps are not
	// internally synchronized while multiple server loops report connection
	// changes concurrently.
	internalTrackerMtx sync.Mutex

	totalByID map[string][]*salmon.ItemWContext
	totalMtx  sync.Mutex
	// clients are retained so Close can stop every per-server connection.
	clients []*WSClient
	// tunnels are retained so their commands are stopped with the combiner.
	tunnels []*TunnelSupervisor
	// closeOnce and closeDone coordinate shutdown between all combiner loops.
	closeOnce sync.Once
	closeDone chan struct{}
	// wg tracks the per-server loops that consume client events.
	wg sync.WaitGroup
}

type CombinerParams struct {
	Config Config
	Logger *logs.Logger

	OngoingIncidentsHandler func(notif *salmon.Notification)

	Clock          clock.Clock
	ReconnectDelay time.Duration
	// ConnectionStatusHandler receives per-host connection and heartbeat data.
	ConnectionStatusHandler func(id string, event ConnectionEvent)
}

func NewCombiner(params CombinerParams) (*Combiner, error) {
	if err := params.Config.Validate(); err != nil {
		return nil, err
	}
	if params.Logger == nil {
		panic("Logger is required")
	}
	params.Logger = params.Logger.WithNamespaceAppended("Combiner")

	c := &Combiner{
		params: params,

		totalByID: map[string][]*salmon.ItemWContext{},
		closeDone: make(chan struct{}),
	}

	c.internalTracker = statestracker.NewItemStatesTracker(statestracker.ItemStatesTrackerParams{
		Clock: params.Clock,
	})

	for i, cfg := range c.params.Config.Servers {
		serverEventCh := make(chan ServerEvent, 32)

		command, err := TunnelCommand(cfg)
		if err != nil {
			c.Close()
			return nil, errors.Annotatef(err, "creating tunnel command #%d (%s)", i, cfg.ID)
		}
		var tunnel *TunnelSupervisor
		if command != nil {
			tunnel = NewTunnelSupervisor(TunnelSupervisorParams{
				ServerID: cfg.ID,
				Command:  *command,
				Logger:   params.Logger,
				EventCh:  serverEventCh,
			})
			c.tunnels = append(c.tunnels, tunnel)
		}

		wsc, err := New(Params{
			Config: cfg,
			Logger: params.Logger,

			EventCh:        serverEventCh,
			ReconnectDelay: params.ReconnectDelay,
			Tunnel:         tunnel,
		})
		if err != nil {
			c.Close()
			return nil, errors.Annotatef(err, "creating wsclient #%d (%s)", i, cfg.ID)
		}

		c.clients = append(c.clients, wsc)
		c.wg.Add(1)
		go func(
			cfg ConfigServer,
			serverEventCh <-chan ServerEvent,
		) {
			defer c.wg.Done()
			c.runWSClient(cfg, serverEventCh)
		}(cfg, serverEventCh)
	}

	return c, nil
}

// Close stops all Salmon connections and waits for their combiner loops.
func (c *Combiner) Close() {
	c.closeOnce.Do(func() {
		c.params.Logger.Log(logs.Info, "Shutting down")
		close(c.closeDone)
		for _, client := range c.clients {
			client.Close()
		}
		for _, tunnel := range c.tunnels {
			tunnel.Close()
		}
		c.wg.Wait()
		c.params.Logger.Log(logs.Info, "Shutdown complete")
	})
}

func (c *Combiner) applyNotification(id string, notif *salmon.Notification) {
	c.totalMtx.Lock()
	defer c.totalMtx.Unlock()

	c.totalByID[id] = notif.OngoingIncidents.Total
	total := c.combinedTotalLocked()

	notifCombined := &salmon.Notification{
		Time: notif.Time,

		OngoingIncidents: salmon.OngoingIncidentsWDelta{
			Total: total,

			Added:   notif.OngoingIncidents.Added,
			Removed: notif.OngoingIncidents.Removed,
			Updated: notif.OngoingIncidents.Updated,

			NumItemsOK: notif.OngoingIncidents.NumItemsOK,
		},
	}

	if c.params.OngoingIncidentsHandler != nil {
		c.params.OngoingIncidentsHandler(notifCombined)
	}
}

// combinedTotalLocked returns all cached server snapshots in stable server-ID
// order. The caller must hold totalMtx.
func (c *Combiner) combinedTotalLocked() []*salmon.ItemWContext {
	ids := make([]string, 0, len(c.totalByID))
	fullLen := 0
	for id, items := range c.totalByID {
		ids = append(ids, id)
		fullLen += len(items)
	}

	sort.Strings(ids)

	total := make([]*salmon.ItemWContext, 0, fullLen)
	for _, id := range ids {
		total = append(total, c.totalByID[id]...)
	}
	return total
}

// markServerIncidentsStale replaces a server's cached incidents with stale
// copies and publishes the resulting combined snapshot. Repeated disconnects
// do not publish another update when everything is already stale.
func (c *Combiner) markServerIncidentsStale(id string, eventTime time.Time) {
	c.totalMtx.Lock()
	defer c.totalMtx.Unlock()

	items := c.totalByID[id]
	updated := make([]*salmon.ItemWContext, 0, len(items))
	changedItems := make([]*salmon.ItemWContext, 0, len(items))
	for _, item := range items {
		if item == nil {
			updated = append(updated, nil)
			continue
		}
		itemCopy := *item
		if !itemCopy.Stale {
			itemCopy.Stale = true
			changedItems = append(changedItems, &itemCopy)
		}
		updated = append(updated, &itemCopy)
	}
	if len(changedItems) == 0 {
		return
	}

	c.totalByID[id] = updated
	if c.params.OngoingIncidentsHandler != nil {
		c.params.OngoingIncidentsHandler(&salmon.Notification{
			Time: eventTime,
			OngoingIncidents: salmon.OngoingIncidentsWDelta{
				Total:   c.combinedTotalLocked(),
				Updated: changedItems,
			},
		})
	}
}

// ForgetStaleIncident removes a stale incident from the cached source snapshot
// and publishes the new combined total. The notification intentionally has no
// Removed delta: forgetting is a local dismissal, not evidence that the
// incident was resolved. A later source snapshot can report the incident again.
func (c *Combiner) ForgetStaleIncident(key string) bool {
	c.totalMtx.Lock()
	defer c.totalMtx.Unlock()

	for id, items := range c.totalByID {
		for i, item := range items {
			if item == nil || string(item.Key) != key || !item.Stale {
				continue
			}

			updated := make([]*salmon.ItemWContext, 0, len(items)-1)
			updated = append(updated, items[:i]...)
			updated = append(updated, items[i+1:]...)
			c.totalByID[id] = updated

			c.params.Logger.Log(logs.Info, "Forgot stale incident %s", key)
			if c.params.OngoingIncidentsHandler != nil {
				c.params.OngoingIncidentsHandler(&salmon.Notification{
					Time: c.params.Clock.Now(),
					OngoingIncidents: salmon.OngoingIncidentsWDelta{
						Total: c.combinedTotalLocked(),
					},
				})
			}
			return true
		}
	}

	return false
}

func (c *Combiner) runWSClient(
	cfg ConfigServer,
	serverEventCh <-chan ServerEvent,
) {
	for {
		select {
		case <-c.closeDone:
			return
		case event := <-serverEventCh:
			c.applyServerEvent(cfg.ID, event)
		}
	}
}

func (c *Combiner) applyServerEvent(id string, event ServerEvent) {
	switch event.Kind {
	case ServerEventKindConnection:
		if c.params.ConnectionStatusHandler != nil {
			c.params.ConnectionStatusHandler(id, event.Connection)
		}
		if event.Connection.EventKind == EventKindDisconnected {
			c.markServerIncidentsStale(id, event.Connection.Time)
		}

	case ServerEventKindOngoingIncidents:
		notif, err := getPrefixedNotif(event.OngoingIncidents, id)
		if err != nil {
			c.params.Logger.Log(logs.Error, "Ignoring invalid incident notification from %s: %s", id, err)
			return
		}
		c.applyNotification(id, notif)

	case ServerEventKindConnectionError:
		c.applyInternalItem(salmon.ItemKey(fmt.Sprintf("internal.connection.%s", id)), event.ConnectionError)

	case ServerEventKindTunnel:
		err := event.Tunnel.Error
		if event.Tunnel.Kind == TunnelEventReady {
			err = ""
		} else {
			connectionEvent := ConnectionEvent{EventKind: EventKindDisconnected, Time: event.Tunnel.Time}
			if c.params.ConnectionStatusHandler != nil {
				c.params.ConnectionStatusHandler(id, connectionEvent)
			}
			c.markServerIncidentsStale(id, event.Tunnel.Time)
		}
		c.applyInternalItem(salmon.ItemKey(fmt.Sprintf("internal.tunnel.%s", id)), err)

	default:
		c.params.Logger.Log(logs.Error, "Ignoring invalid event kind %q from %s", event.Kind, id)
	}
}

func (c *Combiner) applyInternalItem(key salmon.ItemKey, err string) {
	state := salmon.ItemStateOK
	if err != "" {
		state = salmon.ItemStateError
	}

	c.internalTrackerMtx.Lock()
	notif := c.internalTracker.FeedItems(map[salmon.ItemKey]*salmon.Item{
		key: &salmon.Item{
			Key:     key,
			State:   state,
			Details: err,
		},
	})
	c.internalTrackerMtx.Unlock()

	// If nothing has changed, we're done.
	if notif == nil {
		return
	}

	c.applyNotification(IDInternal, notif)
}

func getPrefixedNotif(notif *salmon.Notification, prefix string) (*salmon.Notification, error) {
	if err := validateNotification(notif); err != nil {
		return nil, err
	}

	return &salmon.Notification{
		Time: notif.Time,

		OngoingIncidents: salmon.OngoingIncidentsWDelta{
			Total: getPrefixedItems(notif.OngoingIncidents.Total, prefix),

			Added:   getPrefixedItems(notif.OngoingIncidents.Added, prefix),
			Removed: getPrefixedItems(notif.OngoingIncidents.Removed, prefix),
			Updated: getPrefixedItems(notif.OngoingIncidents.Updated, prefix),

			NumItemsOK: notif.OngoingIncidents.NumItemsOK,
		},
	}, nil
}

func getPrefixedItems(items []*salmon.ItemWContext, prefix string) []*salmon.ItemWContext {
	ret := make([]*salmon.ItemWContext, 0, len(items))
	for _, item := range items {
		ret = append(ret, getPrefixedItem(item, prefix))
	}

	return ret
}

func getPrefixedItem(item *salmon.ItemWContext, prefix string) *salmon.ItemWContext {
	return &salmon.ItemWContext{
		Item: salmon.Item{
			Key:     salmon.ItemKey(prefix + "." + string(item.Key)),
			State:   item.State,
			Details: item.Details,
		},

		IncidentStartedAt: item.IncidentStartedAt,
		Stale:             item.Stale,
	}
}
