package wsclient

import (
	"fmt"
	"sort"
	"sync"

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

	totalByID map[string][]*salmon.ItemWContext
	totalMtx  sync.Mutex
}

type CombinerParams struct {
	Config Config

	OngoingIncidentsHandler func(notif *salmon.Notification)
	//AuthnHandler               func(user, err string)
	//ServerInternalErrorHandler func(err string)

	Clock clock.Clock
}

func NewCombiner(params CombinerParams) (*Combiner, error) {
	c := &Combiner{
		params: params,

		totalByID: map[string][]*salmon.ItemWContext{},
	}

	c.internalTracker = statestracker.NewItemStatesTracker(statestracker.ItemStatesTrackerParams{
		Clock: params.Clock,
	})

	for i, cfg := range c.params.Config.Servers {
		ongoingIncidentsCh := make(chan *salmon.Notification, 32)
		connErrorCh := make(chan string, 32)
		serverInternalErrorCh := make(chan string, 32)

		go c.runWSClient(cfg, ongoingIncidentsCh, connErrorCh, serverInternalErrorCh)

		wsc, err := New(Params{
			Config: cfg,

			OngoingIncidentsCh:    ongoingIncidentsCh,
			ConnErrorCh:           connErrorCh,
			ServerInternalErrorCh: serverInternalErrorCh,
		})
		if err != nil {
			return nil, errors.Annotatef(err, "creating wsclient #%d (%s)", i, cfg.ID)
		}

		_ = wsc
	}

	return c, nil
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
) {
	// TODO: implement teardown
	for {
		select {
		case notif := <-ongoingIncidentsCh:
			notif = getPrefixedNotif(notif, cfg.ID)

			c.applyNotification(cfg.ID, notif)

		case err := <-connErrorCh:
			connKey := salmon.ItemKey(fmt.Sprintf("internal.connection.%s", cfg.ID))
			state := salmon.ItemStateOK
			if err != "" {
				state = salmon.ItemStateError
			}

			notif := c.internalTracker.FeedItems(map[salmon.ItemKey]*salmon.Item{
				connKey: &salmon.Item{
					Key:     connKey,
					State:   state,
					Comment: err,
				},
			})

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
