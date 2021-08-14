package itemsboard

import (
	"sync"

	"github.com/dimonomid/salmon"
)

type ItemsBoard struct {
	items []*salmon.ItemWContext

	mtx sync.RWMutex
}

func New() *ItemsBoard {
	return &ItemsBoard{}
}

func (ib *ItemsBoard) Set(items []*salmon.ItemWContext) {
	ib.mtx.Lock()
	defer ib.mtx.Unlock()

	ib.items = items
}

func (ib *ItemsBoard) Get() []*salmon.ItemWContext {
	ib.mtx.RLock()
	defer ib.mtx.RUnlock()

	return ib.items
}
