package collectors

import (
	"github.com/dimonomid/salmon"
)

type Collector interface {
	// Close tears down the collector. It's synchronous: by the time Close returns,
	// all resources held by the Collector are already reclaimed.
	Close()
}

type Params struct {
	ID string

	UpdatesChan chan<- *Update
}

type Update struct {
	Items map[salmon.ItemKey]*salmon.Item
	Err   error
}
