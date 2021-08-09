package salmon

import "time"

type Item struct {
	// Key uniquely identifies the item.
	Key ItemKey

	// State is the current item's state.
	State ItemState

	// Comment, if not empty, contains a human-readable clarification on why
	// State is what it is.
	Comment string
}

type ItemKey string

type ItemState string

const (
	ItemStateOK      ItemState = "ok"
	ItemStateWarning ItemState = "warning"
	ItemStateError   ItemState = "error"
)

type ItemWContext struct {
	Item

	// ChangeTime indicates when the item entered the ItemStateOK or
	// non-ItemStateOK state.
	ChangeTime time.Time
}

func (item *Item) Equals(other *Item) bool {
	return item.Key == other.Key && item.State == other.State && item.Comment == other.Comment
}
