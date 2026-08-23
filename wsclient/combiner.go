package wsclient

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dimonomid/salmon"
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
	// closeOnce and closeDone coordinate shutdown between all combiner loops.
	closeOnce sync.Once
	closeDone chan struct{}
	// wg tracks the per-server loops that consume client events.
	wg sync.WaitGroup
}

type CombinerParams struct {
	Config Config

	OngoingIncidentsHandler func(notif *salmon.Notification)
	//ServerInternalErrorHandler func(err string)

	Clock          clock.Clock
	ReconnectDelay time.Duration
	// ConnectionStatusHandler receives per-host connection and heartbeat data.
	ConnectionStatusHandler func(id string, event ConnectionEvent)
}

func NewCombiner(params CombinerParams) (*Combiner, error) {
	if err := params.Config.Validate(); err != nil {
		return nil, err
	}

	c := &Combiner{
		params: params,

		totalByID: map[string][]*salmon.ItemWContext{},
		closeDone: make(chan struct{}),
	}

	c.internalTracker = statestracker.NewItemStatesTracker(statestracker.ItemStatesTrackerParams{
		Clock: params.Clock,
	})

	for i, cfg := range c.params.Config.Servers {
		ongoingIncidentsCh := make(chan *salmon.Notification, 32)
		connErrorCh := make(chan string, 32)
		serverInternalErrorCh := make(chan string, 32)
		connectionEventCh := make(chan ConnectionEvent, 32)

		wsc, err := New(Params{
			Config: cfg,

			OngoingIncidentsCh:    ongoingIncidentsCh,
			ConnErrorCh:           connErrorCh,
			ServerInternalErrorCh: serverInternalErrorCh,
			ReconnectDelay:        params.ReconnectDelay,
			ConnectionEventCh:     connectionEventCh,
		})
		if err != nil {
			c.Close()
			return nil, errors.Annotatef(err, "creating wsclient #%d (%s)", i, cfg.ID)
		}

		c.clients = append(c.clients, wsc)
		c.wg.Add(1)
		go func(
			cfg ConfigServer,
			ongoingIncidentsCh <-chan *salmon.Notification,
			connErrorCh <-chan string,
			serverInternalErrorCh <-chan string,
			connectionEventCh <-chan ConnectionEvent,
		) {
			defer c.wg.Done()
			c.runWSClient(cfg, ongoingIncidentsCh, connErrorCh, serverInternalErrorCh, connectionEventCh)
		}(cfg, ongoingIncidentsCh, connErrorCh, serverInternalErrorCh, connectionEventCh)
	}

	return c, nil
}

// Close stops all Salmon connections and waits for their combiner loops.
func (c *Combiner) Close() {
	c.closeOnce.Do(func() {
		close(c.closeDone)
		for _, client := range c.clients {
			client.Close()
		}
		c.wg.Wait()
	})
}

func (c *Combiner) applyNotification(id string, notif *salmon.Notification) {
	c.totalMtx.Lock()
	defer c.totalMtx.Unlock()

	c.totalByID[id] = notif.OngoingIncidents.Total

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

func (c *Combiner) runWSClient(
	cfg ConfigServer,
	ongoingIncidentsCh <-chan *salmon.Notification,
	connErrorCh <-chan string,
	serverInternalErrorCh <-chan string,
	connectionEventCh <-chan ConnectionEvent,
) {
	for {
		select {
		case <-c.closeDone:
			return
		case event := <-connectionEventCh:
			if c.params.ConnectionStatusHandler != nil {
				c.params.ConnectionStatusHandler(cfg.ID, event)
			}
		case notif := <-ongoingIncidentsCh:
			notif = getPrefixedNotif(notif, cfg.ID)

			c.applyNotification(cfg.ID, notif)

		case err := <-connErrorCh:
			connKey := salmon.ItemKey(fmt.Sprintf("internal.connection.%s", cfg.ID))
			state := salmon.ItemStateOK
			if err != "" {
				state = salmon.ItemStateError
			}

			c.internalTrackerMtx.Lock()
			notif := c.internalTracker.FeedItems(map[salmon.ItemKey]*salmon.Item{
				connKey: &salmon.Item{
					Key:     connKey,
					State:   state,
					Comment: err,
				},
			})
			c.internalTrackerMtx.Unlock()

			// If nothing has changed, we're done.
			if notif == nil {
				break
			}

			c.applyNotification(IDInternal, notif)

		case err := <-serverInternalErrorCh:
			// TODO
			_ = err
			//if c.params.ServerInternalErrorHandler != nil {
			//c.params.ServerInternalErrorHandler(err)
			//}
		}
	}
}

func getPrefixedNotif(notif *salmon.Notification, prefix string) *salmon.Notification {
	return &salmon.Notification{
		Time: notif.Time,

		OngoingIncidents: salmon.OngoingIncidentsWDelta{
			Total: getPrefixedItems(notif.OngoingIncidents.Total, prefix),

			Added:   getPrefixedItems(notif.OngoingIncidents.Added, prefix),
			Removed: getPrefixedItems(notif.OngoingIncidents.Removed, prefix),
			Updated: getPrefixedItems(notif.OngoingIncidents.Updated, prefix),

			NumItemsOK: notif.OngoingIncidents.NumItemsOK,
		},
	}
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
			Comment: item.Comment,
		},

		ChangeTime: item.ChangeTime,
	}
}
