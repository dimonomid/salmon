package itemsboard_test

import (
	"testing"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/itemsboard"
)

func TestBoardPublishesLatestSnapshot(t *testing.T) {
	board := itemsboard.New()
	if got := board.Get(); len(got) != 0 {
		t.Fatalf("new board contains %#v", got)
	}

	want := []*salmon.ItemWContext{{
		Item:              salmon.Item{Key: "disk.free", State: salmon.ItemStateError, Details: "full"},
		IncidentStartedAt: time.Unix(123, 0),
	}}
	board.Set(want)
	got := board.Get()
	if len(got) != 1 || !got[0].Item.Equals(&want[0].Item) || !got[0].IncidentStartedAt.Equal(want[0].IncidentStartedAt) {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
}
