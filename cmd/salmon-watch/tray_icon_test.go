package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/wsclient"
)

func TestComposeIconRespectsOverlayTransparency(t *testing.T) {
	base := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			base.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}

	overlay := image.NewRGBA(image.Rect(0, 0, 4, 4))
	// The source pixel mapped to the lower-right output pixel is half-opaque
	// red. Other mapped pixels stay transparent.
	overlay.Set(0, 0, color.RGBA{R: 255, A: 128})

	result, err := png.Decode(bytes.NewReader(composeIcon(encodePNG(t, base), encodePNG(t, overlay))))
	if err != nil {
		t.Fatal(err)
	}

	if got := color.RGBAModel.Convert(result.At(2, 2)).(color.RGBA); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("transparent overlay pixel erased the main icon: %#v", got)
	}
	got := color.RGBAModel.Convert(result.At(3, 3)).(color.RGBA)
	if got.A != 255 || got.R == 0 || got.B == 0 {
		t.Fatalf("semi-transparent overlay was not alpha-composited: %#v", got)
	}
}

func TestIconCombinerCachesComposedIcons(t *testing.T) {
	base := image.NewRGBA(image.Rect(0, 0, 4, 4))
	base.Set(0, 0, color.RGBA{A: 255})
	icon := encodePNG(t, base)
	combiner := &iconCombiner{
		icons: map[overallState][]byte{
			overallStateWarning: icon,
			overallStateError:   icon,
		},
		composedIcons: make(map[trayIconKey][]byte),
	}
	snoozed := overallStateError
	state := trayState{Alerting: overallStateWarning, Snoozed: &snoozed}

	combiner.Icon(state)
	combiner.Icon(state)

	if got := len(combiner.composedIcons); got != 1 {
		t.Fatalf("got %d cached composed icons, want 1", got)
	}
}

func TestUnknownTrayIconIsGrayscaleWithOKIconAlpha(t *testing.T) {
	okIcon, err := png.Decode(bytes.NewReader(mustEmbeddedAsset("assets/salmon_green.png")))
	if err != nil {
		t.Fatal(err)
	}
	unknownIcon, err := png.Decode(bytes.NewReader(mustEmbeddedAsset("assets/salmon_gray.png")))
	if err != nil {
		t.Fatal(err)
	}
	if okIcon.Bounds() != unknownIcon.Bounds() {
		t.Fatalf("unknown icon bounds = %v, want %v", unknownIcon.Bounds(), okIcon.Bounds())
	}

	for y := okIcon.Bounds().Min.Y; y < okIcon.Bounds().Max.Y; y++ {
		for x := okIcon.Bounds().Min.X; x < okIcon.Bounds().Max.X; x++ {
			_, _, _, okAlpha := okIcon.At(x, y).RGBA()
			red, green, blue, unknownAlpha := unknownIcon.At(x, y).RGBA()
			if unknownAlpha != okAlpha {
				t.Fatalf("unknown icon alpha at (%d, %d) = %d, want %d", x, y, unknownAlpha, okAlpha)
			}
			if red != green || green != blue {
				t.Fatalf("unknown icon pixel at (%d, %d) is not gray", x, y)
			}
		}
	}
}

func TestTrayStatusTitle(t *testing.T) {
	if got, want := trayStatusTitle(trayState{}), "Status: 0 incidents"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := trayStatusTitle(trayState{AlertingCount: 1}), "Status: 1 incident"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := trayStatusTitle(trayState{AlertingCount: 2, SnoozedCount: 1}), "Status: 2 incidents + 1 snoozed"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestOnIncidentUpdateDerivesTrayState(t *testing.T) {
	var received []trayState
	core := &salmonWatchCore{
		statusWebserver: newStatusWebserver(statusWebserverParams{Logger: watchTestLogger}),
		onIconState: func(state trayState) {
			received = append(received, state)
		},
	}

	core.onIncidentUpdate(incidentSnapshot{
		Alerting: []salmon.ItemWContext{{
			Item: salmon.Item{State: salmon.ItemStateWarning},
		}},
		Snoozed: []snoozedIncident{{
			ItemWContext: salmon.ItemWContext{Item: salmon.Item{State: salmon.ItemStateError}},
			SnoozedUntil: time.Now().Add(time.Hour),
		}},
	})
	if len(received) != 1 {
		t.Fatalf("got %d tray updates, want 1", len(received))
	}
	if received[0].Alerting != overallStateWarning || received[0].AlertingCount != 1 || received[0].Snoozed == nil || *received[0].Snoozed != overallStateError || received[0].SnoozedCount != 1 {
		t.Fatalf("unexpected tray state: %#v", received[0])
	}

	core.onIncidentUpdate(incidentSnapshot{})
	if len(received) != 2 || received[1].Alerting != overallStateOK || received[1].AlertingCount != 0 || received[1].Snoozed != nil || received[1].SnoozedCount != 0 {
		t.Fatalf("unexpected empty tray state: %#v", received)
	}
}

func TestOverallStateRemainsUnknownUntilEveryServerHasAnOutcome(t *testing.T) {
	var received []trayState
	core := &salmonWatchCore{
		incidentState: &incidentState{
			snoozes: &snoozeState{snoozed: make(map[string]snoozeEntry)},
			clock:   clock.New(),
		},
		statusWebserver: newStatusWebserver(statusWebserverParams{Logger: watchTestLogger}),
		onIconState: func(state trayState) {
			received = append(received, state)
		},
		serverStatuses: map[string]serverStatus{
			"first":  {ID: "first"},
			"second": {ID: "second"},
			"third":  {ID: "third"},
		},
		serverIDs: []string{"first", "second", "third"},
	}
	core.incidentState.OnUpdate = core.onIncidentUpdate

	core.onConnectionEvent("first", wsclient.ConnectionEvent{
		EventKind: wsclient.EventKindConnected,
		Time:      time.Now(),
	})
	if got := received[len(received)-1].Alerting; got != overallStateUnknown {
		t.Fatalf("state after first server connects = %s, want unknown", got)
	}

	core.onConnectionEvent("second", wsclient.ConnectionEvent{
		EventKind: wsclient.EventKindDisconnected,
		Time:      time.Now(),
	})
	if !core.serverStatuses["second"].hasConnectedOrFailed {
		t.Fatal("failed connection was not recorded as an observed outcome")
	}

	connectionIncident := &salmon.ItemWContext{
		Item: salmon.Item{Key: "internal.connection.second", State: salmon.ItemStateError},
	}
	core.incidentState.Update([]*salmon.ItemWContext{connectionIncident})
	if got := received[len(received)-1].Alerting; got != overallStateInternalError {
		t.Fatalf("state with a connection failure = %s, want internal error", got)
	}

	core.onConnectionEvent("second", wsclient.ConnectionEvent{
		EventKind: wsclient.EventKindConnected,
		Time:      time.Now(),
	})
	core.incidentState.Update(nil)
	if got := received[len(received)-1].Alerting; got != overallStateUnknown {
		t.Fatalf("state after second server reconnects with third pending = %s, want unknown", got)
	}

	core.onConnectionEvent("third", wsclient.ConnectionEvent{
		EventKind: wsclient.EventKindConnected,
		Time:      time.Now(),
	})
	if got := received[len(received)-1].Alerting; got != overallStateOK {
		t.Fatalf("state after every server responds = %s, want ok", got)
	}
}

func encodePNG(t *testing.T, source image.Image) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
