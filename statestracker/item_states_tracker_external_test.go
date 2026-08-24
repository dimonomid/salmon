package statestracker_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/statestracker"
)

func TestTrackerPublishesIncidentLifecycle(t *testing.T) {
	mockClock := clock.NewMock()
	tracker := statestracker.NewItemStatesTracker(statestracker.ItemStatesTrackerParams{Clock: mockClock})
	key := salmon.ItemKey("systemd.sync.service")

	if got := tracker.FeedItems(items(item(key, salmon.ItemStateOK, "active"))); got != nil {
		t.Fatalf("initial healthy observation produced notification %#v", got)
	}

	mockClock.Add(time.Minute)
	addedAt := mockClock.Now()
	added := tracker.FeedItems(items(item(key, salmon.ItemStateWarning, "failed")))
	assertNotification(t, added, []string{string(key)}, []string{string(key)}, nil, nil, 0)
	if got := added.OngoingIncidents.Total[0].ChangeTime; !got.Equal(addedAt) {
		t.Fatalf("incident ChangeTime = %s, want %s", got, addedAt)
	}

	mockClock.Add(time.Minute)
	updated := tracker.FeedItems(items(item(key, salmon.ItemStateError, "still failed")))
	assertNotification(t, updated, []string{string(key)}, nil, nil, []string{string(key)}, 0)
	if got := updated.OngoingIncidents.Total[0].ChangeTime; !got.Equal(addedAt) {
		t.Fatalf("updated incident ChangeTime = %s, want original %s", got, addedAt)
	}

	if got := tracker.FeedItems(items(item(key, salmon.ItemStateError, "still failed"))); got != nil {
		t.Fatalf("redundant observation produced notification %#v", got)
	}

	mockClock.Add(time.Minute)
	removed := tracker.FeedItems(items(item(key, salmon.ItemStateOK, "active")))
	assertNotification(t, removed, nil, nil, []string{string(key)}, nil, 1)
}

func TestTrackerSortsSnapshotsAndKeepsIndependentItems(t *testing.T) {
	tracker := statestracker.NewItemStatesTracker(statestracker.ItemStatesTrackerParams{Clock: clock.NewMock()})
	notification := tracker.FeedItems(items(
		item("z.last", salmon.ItemStateError, "z"),
		item("a.first", salmon.ItemStateWarning, "a"),
		item("m.healthy", salmon.ItemStateOK, "m"),
	))
	assertNotification(t, notification,
		[]string{"a.first", "z.last"},
		[]string{"a.first", "z.last"}, nil, nil, 1,
	)
}

func TestPublishedNotificationIsNotChangedByLaterUpdates(t *testing.T) {
	tracker := statestracker.NewItemStatesTracker(statestracker.ItemStatesTrackerParams{Clock: clock.NewMock()})
	key := salmon.ItemKey("probe")
	first := tracker.FeedItems(items(item(key, salmon.ItemStateWarning, "first failure")))

	tracker.FeedItems(items(item(key, salmon.ItemStateError, "failure changed")))

	assertItem := func(name string, got *salmon.ItemWContext) {
		t.Helper()
		if got.State != salmon.ItemStateWarning || got.Comment != "first failure" {
			t.Errorf("%s after later update = state %q, comment %q; want state %q, comment %q",
				name, got.State, got.Comment, salmon.ItemStateWarning, "first failure")
		}
	}
	assertItem("Total item", first.OngoingIncidents.Total[0])
	assertItem("Added item", first.OngoingIncidents.Added[0])
}

func item(key salmon.ItemKey, state salmon.ItemState, comment string) *salmon.Item {
	return &salmon.Item{Key: key, State: state, Comment: comment}
}

func items(values ...*salmon.Item) map[salmon.ItemKey]*salmon.Item {
	result := make(map[salmon.ItemKey]*salmon.Item, len(values))
	for _, value := range values {
		result[value.Key] = value
	}
	return result
}

func assertNotification(t *testing.T, notification *salmon.Notification, total, added, removed, updated []string, healthy int) {
	t.Helper()
	if notification == nil {
		t.Fatal("notification is nil")
	}
	incidentKeys := func(values []*salmon.ItemWContext) []string {
		keys := make([]string, 0, len(values))
		for _, value := range values {
			keys = append(keys, string(value.Key))
		}
		return keys
	}
	normalize := func(values []string) []string {
		if values == nil {
			return []string{}
		}
		return values
	}
	got := notification.OngoingIncidents
	if keys := incidentKeys(got.Total); !reflect.DeepEqual(keys, normalize(total)) {
		t.Errorf("Total keys = %#v, want %#v", keys, total)
	}
	if keys := incidentKeys(got.Added); !reflect.DeepEqual(keys, normalize(added)) {
		t.Errorf("Added keys = %#v, want %#v", keys, added)
	}
	if keys := incidentKeys(got.Removed); !reflect.DeepEqual(keys, normalize(removed)) {
		t.Errorf("Removed keys = %#v, want %#v", keys, removed)
	}
	if keys := incidentKeys(got.Updated); !reflect.DeepEqual(keys, normalize(updated)) {
		t.Errorf("Updated keys = %#v, want %#v", keys, updated)
	}
	if got.NumItemsOK != healthy {
		t.Errorf("NumItemsOK = %d, want %d", got.NumItemsOK, healthy)
	}
}
