package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/dimonomid/salmon"
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
		statusWebserver: newStatusWebserver(statusWebserverParams{}),
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

func encodePNG(t *testing.T, source image.Image) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
