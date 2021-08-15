package statestracker

import (
	"sort"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon"
)

type ItemStatesTracker struct {
	params ItemStatesTrackerParams

	itemsOK    map[salmon.ItemKey]*salmon.ItemWContext
	itemsNotOK map[salmon.ItemKey]*salmon.ItemWContext
}

type ItemStatesTrackerParams struct {
	Clock clock.Clock
}

func NewItemStatesTracker(params ItemStatesTrackerParams) *ItemStatesTracker {
	return &ItemStatesTracker{
		params: params,

		itemsOK:    map[salmon.ItemKey]*salmon.ItemWContext{},
		itemsNotOK: map[salmon.ItemKey]*salmon.ItemWContext{},
	}
}

func (ist *ItemStatesTracker) FeedItems(newItems map[salmon.ItemKey]*salmon.Item) *salmon.Notification {
	added := map[salmon.ItemKey]*salmon.ItemWContext{}
	removed := map[salmon.ItemKey]*salmon.ItemWContext{}
	updated := map[salmon.ItemKey]*salmon.ItemWContext{}

	for _, item := range newItems {
		if item.State == salmon.ItemStateOK {
			if _, exists := ist.itemsOK[item.Key]; !exists {
				// Either it's a new item, or it transitioned from non-ok to ok.
				iwc := &salmon.ItemWContext{
					Item:       *item,
					ChangeTime: ist.params.Clock.Now(),
				}
				ist.itemsOK[item.Key] = iwc
			}

			if exItem, exists := ist.itemsNotOK[item.Key]; exists {
				delete(ist.itemsNotOK, item.Key)

				// Add a delta that an incident was removed.
				removed[item.Key] = exItem
			}
		} else {
			if exItem, exists := ist.itemsNotOK[item.Key]; !exists {
				// Either it's a new item, or it transitioned from ok to non-ok.
				iwc := &salmon.ItemWContext{
					Item:       *item,
					ChangeTime: ist.params.Clock.Now(),
				}
				ist.itemsNotOK[item.Key] = iwc

				// Add a delta that an incident was added.
				added[item.Key] = iwc
			} else if !exItem.Item.Equals(item) {
				// Incident existed already, and its details have changed, add a delta
				// with that.
				exItem.Item = *item
				updated[item.Key] = exItem
			}

			if _, exists := ist.itemsOK[item.Key]; exists {
				delete(ist.itemsOK, item.Key)
			}
		}
	}

	// If there were no changes, just return nil.
	if len(added) == 0 && len(removed) == 0 && len(updated) == 0 {
		return nil
	}

	// There were some changes, so return them.

	notif := salmon.Notification{
		Time: ist.params.Clock.Now(),

		OngoingIncidents: salmon.OngoingIncidentsWDelta{
			Total: itemsMapToSortedSlice(ist.itemsNotOK),

			Added:   itemsMapToSortedSlice(added),
			Removed: itemsMapToSortedSlice(removed),
			Updated: itemsMapToSortedSlice(updated),

			NumItemsOK: len(ist.itemsOK),
		},
	}
	return &notif
}

func itemsMapToSortedSlice(m map[salmon.ItemKey]*salmon.ItemWContext) []*salmon.ItemWContext {
	ret := make([]*salmon.ItemWContext, 0, len(m))
	for _, key := range m {
		ret = append(ret, key)
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Key < ret[j].Key
	})

	return ret
}
