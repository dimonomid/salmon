package wsclient

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/logs"
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

func TestCombinerForgetsOnlyStaleIncidentWithoutResolutionDelta(t *testing.T) {
	clk := clock.NewMock()
	var notifications []*salmon.Notification
	combiner := &Combiner{
		params: CombinerParams{
			Clock:  clk,
			Logger: logs.NewLogger(logs.LoggerParams{Clock: clk}),
			OngoingIncidentsHandler: func(notification *salmon.Notification) {
				notifications = append(notifications, notification)
			},
		},
		totalByID: map[string][]*salmon.ItemWContext{
			"first": {
				combinerTestIncident("first.stale", true),
				combinerTestIncident("first.fresh", false),
			},
			"second": {combinerTestIncident("second.stale", true)},
		},
	}

	if !combiner.ForgetStaleIncident("first.stale") {
		t.Fatal("stale incident was not forgotten")
	}
	if len(notifications) != 1 {
		t.Fatalf("got %d notifications, want 1", len(notifications))
	}
	notification := notifications[0]
	if incidentWithKey(notification.OngoingIncidents.Total, "first.stale") != nil {
		t.Fatalf("forgotten incident remains in total: %#v", notification.OngoingIncidents.Total)
	}
	if incidentWithKey(notification.OngoingIncidents.Total, "first.fresh") == nil ||
		incidentWithKey(notification.OngoingIncidents.Total, "second.stale") == nil {
		t.Fatalf("forget removed an unrelated incident: %#v", notification.OngoingIncidents.Total)
	}
	if len(notification.OngoingIncidents.Added) != 0 ||
		len(notification.OngoingIncidents.Removed) != 0 ||
		len(notification.OngoingIncidents.Updated) != 0 {
		t.Fatalf("forget notification contains a resolution delta: %#v", notification.OngoingIncidents)
	}

	if combiner.ForgetStaleIncident("first.stale") {
		t.Fatal("already-forgotten incident was reported forgotten again")
	}
	if combiner.ForgetStaleIncident("first.fresh") {
		t.Fatal("non-stale incident was forgotten")
	}
	if len(notifications) != 1 {
		t.Fatalf("rejected forget published %d notifications, want 1", len(notifications))
	}

	combiner.applyNotification("first", &salmon.Notification{OngoingIncidents: salmon.OngoingIncidentsWDelta{
		Total: []*salmon.ItemWContext{combinerTestIncident("first.stale", false)},
	}})
	if incident := incidentWithKey(notifications[len(notifications)-1].OngoingIncidents.Total, "first.stale"); incident == nil || incident.Stale {
		t.Fatalf("source snapshot did not restore forgotten incident as fresh: %#v", incident)
	}
}

func TestGetPrefixedItemPreservesStale(t *testing.T) {
	item := getPrefixedItem(combinerTestIncident("disk", true), "bridge")
	if item.Key != "bridge.disk" || !item.Stale {
		t.Fatalf("prefixed incident = %#v, want bridge.disk stale", item)
	}
}

func TestGetPrefixedNotifRejectsNullItem(t *testing.T) {
	_, err := getPrefixedNotif(&salmon.Notification{OngoingIncidents: salmon.OngoingIncidentsWDelta{
		Total: []*salmon.ItemWContext{nil},
	}}, "server")
	if err == nil {
		t.Fatal("null incident was accepted")
	}
}

func TestCombinerReportsTunnelFailureAsInternalIncident(t *testing.T) {
	notifications := make(chan *salmon.Notification, 8)
	combiner, err := NewCombiner(CombinerParams{
		Config: Config{Servers: []ConfigServer{{
			ID:   "remote",
			Addr: "127.0.0.1:41992",
			Tunnel: &ConfigTunnel{CustomCommand: &ConfigCustomTunnelCommand{
				Command:        []string{"sh", "-c", "exit 7"},
				ReadinessProbe: &ConfigTunnelReadinessProbe{ContainsOutput: "ready"},
			}},
		}}},
		Logger: logs.NewLogger(logs.LoggerParams{Clock: clock.New()}),
		Clock:  clock.New(),
		OngoingIncidentsHandler: func(notification *salmon.Notification) {
			notifications <- notification
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(combiner.Close)

	select {
	case notification := <-notifications:
		incident := incidentWithKey(notification.OngoingIncidents.Total, "internal.tunnel.remote")
		if incident == nil || incident.State != salmon.ItemStateError || incident.Details == "" {
			t.Fatalf("notification = %#v, want tunnel failure incident", notification)
		}
		if incidentWithKey(notification.OngoingIncidents.Total, "internal.connection.remote") != nil {
			t.Fatalf("notification = %#v, unexpectedly contains connection incident", notification)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tunnel failure did not produce an internal incident")
	}
}
