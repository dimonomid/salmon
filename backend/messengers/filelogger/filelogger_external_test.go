package filelogger_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/itemsboard"
	"github.com/dimonomid/salmon/backend/messengers"
	"github.com/dimonomid/salmon/backend/messengers/filelogger"
	"github.com/dimonomid/salmon/logs"
)

func TestLoggerWritesObservableIncidentTransitions(t *testing.T) {
	path := t.TempDir() + "/events.log"
	notifications := make(chan *salmon.Notification, 1)
	done := make(chan struct{})
	_, err := filelogger.New(filelogger.Params{
		Common: messengers.Params{
			Logger:            logs.NewLogger(logs.LoggerParams{Clock: clock.New()}),
			ItemsBoard:        itemsboard.New(),
			NotificationsChan: notifications,
			TornDown:          done,
		},
		Config: filelogger.Config{FileName: path},
	})
	if err != nil {
		t.Fatal(err)
	}

	incident := &salmon.ItemWContext{Item: salmon.Item{Key: "systemd.sync", State: salmon.ItemStateError, Details: "failed"}}
	notifications <- &salmon.Notification{
		Time: time.Date(2026, 8, 23, 12, 34, 56, 0, time.Local),
		OngoingIncidents: salmon.OngoingIncidentsWDelta{
			Added:   []*salmon.ItemWContext{incident},
			Updated: []*salmon.ItemWContext{{Item: salmon.Item{Key: "disk.free", State: salmon.ItemStateWarning, Details: "low"}}},
			Removed: []*salmon.ItemWContext{{Item: salmon.Item{Key: "network.dns", State: salmon.ItemStateError}}},
		},
	}
	close(notifications)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("logger did not shut down")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[ ok ] network.dns",
		"[ error ] systemd.sync (failed)",
		"[ warning ][ updated ] disk.free (low)",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log %q does not contain %q", data, want)
		}
	}
}
