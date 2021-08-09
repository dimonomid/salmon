package salmon

import "time"

type Notification struct {
	// Time indicates when the notification was generated.
	Time time.Time

	OngoingIncidents OngoingIncidentsWDelta

	// TODO: add one-off incidents in some form
}

type OngoingIncidentsWDelta struct {
	Total []*ItemWContext

	Added   []*ItemWContext
	Removed []*ItemWContext
	Updated []*ItemWContext

	NumItemsOK int
}
