package wsclient

import (
	"testing"
	"time"

	"github.com/dimonomid/salmon"
)

func combinerTestIncident(key string, stale bool) *salmon.ItemWContext {
	return &salmon.ItemWContext{
		Item:  salmon.Item{Key: salmon.ItemKey(key), State: salmon.ItemStateError},
		Stale: stale,
	}
}

// incidentWithKey finds an incident in a combined snapshot by its prefixed key.
func incidentWithKey(items []*salmon.ItemWContext, key string) *salmon.ItemWContext {
	for _, item := range items {
		if item != nil && string(item.Key) == key {
			return item
		}
	}
	return nil
}

func TestCombinerMarksOnlyDisconnectedServerIncidentsStale(t *testing.T) {
	var notifications []*salmon.Notification
	combiner := &Combiner{
		params: CombinerParams{OngoingIncidentsHandler: func(notification *salmon.Notification) {
			notifications = append(notifications, notification)
		}},
		totalByID: map[string][]*salmon.ItemWContext{
			"first":  {combinerTestIncident("first.disk", false)},
			"second": {combinerTestIncident("second.cpu", false)},
		},
	}
	original := combiner.totalByID["first"][0]
	eventTime := time.Now()

	combiner.markServerIncidentsStale("first", eventTime)
	if len(notifications) != 1 {
		t.Fatalf("got %d notifications, want 1", len(notifications))
	}
	notification := notifications[0]
	if !notification.Time.Equal(eventTime) {
		t.Fatalf("notification time = %v, want %v", notification.Time, eventTime)
	}
	if incident := incidentWithKey(notification.OngoingIncidents.Total, "first.disk"); incident == nil || !incident.Stale {
		t.Fatalf("first.disk = %#v, want stale", incident)
	}
	if incident := incidentWithKey(notification.OngoingIncidents.Total, "second.cpu"); incident == nil || incident.Stale {
		t.Fatalf("second.cpu = %#v, want non-stale", incident)
	}
	if len(notification.OngoingIncidents.Updated) != 1 || !notification.OngoingIncidents.Updated[0].Stale {
		t.Fatalf("updated incidents = %#v, want one stale incident", notification.OngoingIncidents.Updated)
	}
	if original.Stale {
		t.Fatal("marking stale mutated a previously published incident")
	}

	combiner.markServerIncidentsStale("first", eventTime.Add(time.Second))
	if len(notifications) != 1 {
		t.Fatalf("repeated stale mark produced %d notifications, want 1", len(notifications))
	}
}

func TestCombinerSourceSnapshotReplacesStaleIncidents(t *testing.T) {
	var latest *salmon.Notification
	combiner := &Combiner{
		params: CombinerParams{OngoingIncidentsHandler: func(notification *salmon.Notification) {
			latest = notification
		}},
		totalByID: map[string][]*salmon.ItemWContext{
			"first":  {combinerTestIncident("first.disk", true)},
			"second": {combinerTestIncident("second.cpu", true)},
		},
	}

	combiner.applyNotification("first", &salmon.Notification{OngoingIncidents: salmon.OngoingIncidentsWDelta{
		Total: []*salmon.ItemWContext{combinerTestIncident("first.disk", false)},
	}})
	if incident := incidentWithKey(latest.OngoingIncidents.Total, "first.disk"); incident == nil || incident.Stale {
		t.Fatalf("replacement first.disk = %#v, want non-stale", incident)
	}
	if incident := incidentWithKey(latest.OngoingIncidents.Total, "second.cpu"); incident == nil || !incident.Stale {
		t.Fatalf("cached second.cpu = %#v, want stale", incident)
	}

	combiner.applyNotification("second", &salmon.Notification{})
	if incident := incidentWithKey(latest.OngoingIncidents.Total, "second.cpu"); incident != nil {
		t.Fatalf("empty source snapshot retained second.cpu: %#v", incident)
	}
}

func TestGetPrefixedItemPreservesStale(t *testing.T) {
	item := getPrefixedItem(combinerTestIncident("disk", true), "bridge")
	if item.Key != "bridge.disk" || !item.Stale {
		t.Fatalf("prefixed incident = %#v, want bridge.disk stale", item)
	}
}
