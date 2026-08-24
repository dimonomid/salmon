package core

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
	"github.com/dimonomid/salmon/backend/itemsboard"
	"github.com/dimonomid/salmon/statestracker"

	"github.com/benbjohnson/clock"
)

type Core struct {
	params Params

	colls      []collectors.Collector
	messengers []messengerWCtx
	updCh      chan *collectors.Update

	ib *itemsboard.ItemsBoard

	tracker *statestracker.ItemStatesTracker

	torndown  chan struct{}
	closeOnce sync.Once
}

type Params struct {
	Clock clock.Clock
}

func NewCore(cfg Config, params Params) (*Core, error) {
	if params.Clock == nil {
		panic("Clock is required")
	}
	updCh := make(chan *collectors.Update, 16)

	collParams := collectors.Params{
		// NOTE: ID will be populated later for every collector individually.

		UpdatesChan: updCh,
	}

	ib := itemsboard.New()

	colls, err := createCollectors(cfg.Collectors, collParams)
	if err != nil {
		return nil, fmt.Errorf("creating collectors: %w", err)
	}

	messengers, err := createMessengers(cfg.Messengers, ib)
	if err != nil {
		for _, collector := range colls {
			collector.Close()
		}
		return nil, fmt.Errorf("creating messengers: %w", err)
	}

	c := &Core{
		params: params,

		colls:      colls,
		messengers: messengers,
		updCh:      updCh,

		ib: ib,

		tracker: statestracker.NewItemStatesTracker(statestracker.ItemStatesTrackerParams{
			Clock: params.Clock,
		}),

		torndown: make(chan struct{}),
	}

	go c.run()

	return c, nil
}

func (c *Core) Close() {
	c.closeOnce.Do(c.close)
}

func (c *Core) close() {
	// Close all collectors first
	for _, coll := range c.colls {
		coll.Close()
	}

	// At this point we're sure that no more collectors will send any messages to
	// the updCh, so we can close it and wait for the run goroutine to exit as
	// well.
	close(c.updCh)

	// TODO: use a timeout
	<-c.torndown

	// Now that we can't send any more notifications, close all messenger
	// channels and wait for all of them to tear down.
	for _, mwCtx := range c.messengers {
		close(mwCtx.notificationsChan)

		// TODO: use a timeout
		<-mwCtx.tornDown
	}
}

func (c *Core) run() {
	for upd := range c.updCh {
		if upd.Err != nil {
			fmt.Printf("HEY error %+v\n", upd.Err)
			// TODO: convert it to some kind of internal incident
			continue
		}

		//d, err := json.MarshalIndent(upd.Items, "", "  ")
		//if err != nil {
		//panic(err.Error())
		//}
		//fmt.Printf("YO some update: %s\n", string(d))

		notif := c.tracker.FeedItems(upd.Items)
		if notif != nil {
			//d, err := json.MarshalIndent(notif, "", "  ")
			//if err != nil {
			//panic(err.Error())
			//}

			//fmt.Printf("HEY incidents update: %s\n", string(d))

			// We must update items board _before_ sending notifications to the
			// messenger channels; this way, when messengers fan notifications out
			// (e.g. webserver will fan those out to all websocket clients), they can
			// be sure that if they first add a new output channel, then start the
			// goroutine to read from this channel, and that goroutine first of all
			// gets the current status from the ItemsBoard, then it's guaranteed that
			// no updates can be missed. It's possible to get a redundant update
			// instead, but that's ok.
			c.ib.Set(notif.OngoingIncidents.Total)

			for _, mwCtx := range c.messengers {
				sendMessengerNotification(mwCtx, notif)
			}
		} else {
			//fmt.Printf("HEY incidents no-op update\n")
		}
	}

	close(c.torndown)
}

func sendMessengerNotification(mwCtx messengerWCtx, notif *salmon.Notification) {
	select {
	case mwCtx.notificationsChan <- notif:
		return
	default:
	}

	started := time.Now()
	fmt.Fprintf(os.Stderr, "non-blocking notification send to %s did not complete; sending blocking\n", mwCtx.messenger.String())
	mwCtx.notificationsChan <- notif
	fmt.Fprintf(os.Stderr, "blocking notification send to %s completed after %s\n", mwCtx.messenger.String(), time.Since(started))
}
