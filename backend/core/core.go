package core

import (
	"fmt"
	"os"

	"github.com/dimonomid/salmon/backend/collectors"

	"github.com/benbjohnson/clock"
)

type Core struct {
	params Params

	colls      []collectors.Collector
	messengers []messengerWCtx
	updCh      chan *collectors.Update

	tracker *ItemStatesTracker

	torndown chan struct{}
}

type Params struct {
	Clock clock.Clock
}

func NewCore(cfg Config, params Params) (*Core, error) {
	updCh := make(chan *collectors.Update, 16)

	collParams := collectors.Params{
		// NOTE: ID will be populated later for every collector individually.

		UpdatesChan: updCh,
	}

	colls, err := createCollectors(cfg.Collectors, collParams)
	if err != nil {
		return nil, fmt.Errorf("creating collectors: %w", err)
	}

	messengers, err := createMessengers(cfg.Messengers)
	if err != nil {
		return nil, fmt.Errorf("creating messengers: %w", err)
	}

	c := &Core{
		params: params,

		colls:      colls,
		messengers: messengers,
		updCh:      updCh,

		tracker: NewItemStatesTracker(ItemStatesTrackerParams{
			Clock: params.Clock,
		}),

		torndown: make(chan struct{}),
	}

	go c.run()

	return c, nil
}

func (c *Core) Close() {
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
			for _, mwCtx := range c.messengers {
				select {
				case mwCtx.notificationsChan <- notif:
					// All good
				default:
					fmt.Fprintf(os.Stderr, "failed to send notification to %s: buffer is full", mwCtx.messenger.String())
					// TODO: better error handling
				}
			}
		} else {
			//fmt.Printf("HEY incidents no-op update\n")
		}
	}

	close(c.torndown)
}
