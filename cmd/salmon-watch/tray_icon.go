package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"

	"github.com/dimonomid/salmon"
)

var (
	trayIcons *iconCombiner

	iconFlashMtx  sync.Mutex
	iconFlashStop chan struct{}
)

// iconCombiner supplies the base tray icons and composes a snoozed overlay
// when needed. Its cache and lock are private so callers only need to ask for
// the icon that represents a trayState.
type iconCombiner struct {
	icons       map[overallState][]byte
	transparent []byte

	// composedIcons avoids decoding and encoding PNGs for repeated snapshots
	// with the same alerting and snoozed states. It is populated lazily because
	// most state combinations are never needed.
	composedIconsMtx sync.Mutex
	composedIcons    map[trayIconKey][]byte
}

// trayIconKey identifies a cached composition of a main alerting icon and a
// lower-right snoozed overlay icon. It avoids recomposing the same small set of
// state combinations for every incident update.
//
// A key is only constructed when there are snoozed incidents. When there are
// none, no snoozed state sentinel is used: the unmodified alerting icon is
// returned directly instead.
type trayIconKey struct {
	alerting overallState
	snoozed  overallState
}

type overallState int

const (
	overallStateUnknown overallState = iota
	overallStateOK
	overallStateInternalError
	overallStateWarning
	overallStateError
)

func (s overallState) String() string {
	switch s {
	case overallStateUnknown:
		return "unknown"
	case overallStateOK:
		return "ok"
	case overallStateInternalError:
		return "internal error"
	case overallStateWarning:
		return "warning"
	case overallStateError:
		return "error"
	default:
		return fmt.Sprintf("invalid state %d", s)
	}
}

func loadTrayIcons() {
	trayIcons = newIconCombiner()
}

func newIconCombiner() *iconCombiner {
	return &iconCombiner{
		icons: map[overallState][]byte{
			overallStateUnknown:       mustEmbeddedAsset("assets/salmon_gray.png"),
			overallStateOK:            mustEmbeddedAsset("assets/salmon_green.png"),
			overallStateInternalError: mustEmbeddedAsset("assets/salmon_magenta.png"),
			overallStateWarning:       mustEmbeddedAsset("assets/salmon_yellow.png"),
			overallStateError:         mustEmbeddedAsset("assets/salmon_red.png"),
		},
		transparent:   mustEmbeddedAsset("assets/salmon_transparent.png"),
		composedIcons: make(map[trayIconKey][]byte),
	}
}

func getOverallStateFromItems(items []salmon.ItemWContext) overallState {
	ret := overallStateOK

	for _, item := range items {
		cur := getOverallStateFromItem(item)
		if ret < cur {
			ret = cur
		}
	}

	return ret
}

func getOverallStateFromItem(item salmon.ItemWContext) overallState {
	if item.State == salmon.ItemStateOK {
		return overallStateOK
	}

	if strings.HasPrefix(string(item.Key), "internal.") {
		return overallStateInternalError
	}

	if item.State == salmon.ItemStateWarning {
		return overallStateWarning
	}

	return overallStateError
}

func applyIcon(state trayState) {
	icon := trayIcons.Icon(state)

	iconFlashMtx.Lock()
	if iconFlashStop != nil {
		close(iconFlashStop)
		iconFlashStop = nil
	}

	if state.Alerting == overallStateOK {
		systray.SetIcon(icon)
		iconFlashMtx.Unlock()
		return
	}

	stop := make(chan struct{})
	iconFlashStop = stop
	systray.SetIcon(icon)
	iconFlashMtx.Unlock()

	go flashIcon(icon, trayIcons.transparent, stop)
}

// trayStatusTitle summarizes active and snoozed incidents in the tray menu.
func trayStatusTitle(state trayState) string {
	incidentLabel := "incidents"
	if state.AlertingCount == 1 {
		incidentLabel = "incident"
	}
	title := fmt.Sprintf("Status: %d %s", state.AlertingCount, incidentLabel)
	if state.SnoozedCount > 0 {
		title += fmt.Sprintf(" + %d snoozed", state.SnoozedCount)
	}
	return title
}

// Icon returns the tray icon representing state. An icon without a snoozed
// overlay is returned directly; compositions are cached by both states.
func (c *iconCombiner) Icon(state trayState) []byte {
	if state.Snoozed == nil {
		return c.iconForState(state.Alerting)
	}

	key := trayIconKey{alerting: state.Alerting, snoozed: *state.Snoozed}
	c.composedIconsMtx.Lock()
	defer c.composedIconsMtx.Unlock()
	if icon, exists := c.composedIcons[key]; exists {
		return icon
	}

	icon := composeIcon(c.iconForState(state.Alerting), c.iconForState(*state.Snoozed))
	c.composedIcons[key] = icon
	return icon
}

func (c *iconCombiner) iconForState(state overallState) []byte {
	icon, ok := c.icons[state]
	if !ok {
		panic(fmt.Sprintf("invalid state %d", state))
	}
	return icon
}

// composeIcon overlays a state icon, sized to 30% of the main icon, in its
// lower-right corner.
func composeIcon(mainIcon, overlayIcon []byte) []byte {
	base, err := png.Decode(bytes.NewReader(mainIcon))
	if err != nil {
		panic(fmt.Sprintf("decode main tray icon: %s", err))
	}
	overlay, err := png.Decode(bytes.NewReader(overlayIcon))
	if err != nil {
		panic(fmt.Sprintf("decode snoozed tray icon: %s", err))
	}

	bounds := base.Bounds()
	result := image.NewRGBA(bounds)
	draw.Draw(result, bounds, base, bounds.Min, draw.Src)

	size := bounds.Dx() * 30 / 100
	if size == 0 {
		return mainIcon
	}
	overlayBounds := overlay.Bounds()
	scaledOverlay := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sourceX := overlayBounds.Min.X + x*overlayBounds.Dx()/size
			sourceY := overlayBounds.Min.Y + y*overlayBounds.Dy()/size
			scaledOverlay.Set(x, y, overlay.At(sourceX, sourceY))
		}
	}

	startX := bounds.Max.X - size
	startY := bounds.Max.Y - size
	draw.Draw(result, image.Rect(startX, startY, bounds.Max.X, bounds.Max.Y), scaledOverlay, image.Point{}, draw.Over)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, result); err != nil {
		panic(fmt.Sprintf("encode composed tray icon: %s", err))
	}
	return encoded.Bytes()
}

// flashIcon alternates a non-OK icon with a transparent icon until its state
// is replaced by another call to applyIcon.
func flashIcon(icon, transparentIcon []byte, stop <-chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	showIcon := false
	for {
		select {
		case <-ticker.C:
			showIcon = !showIcon

			iconFlashMtx.Lock()
			if iconFlashStop != stop {
				iconFlashMtx.Unlock()
				return
			}
			if showIcon {
				systray.SetIcon(icon)
			} else {
				systray.SetIcon(transparentIcon)
			}
			iconFlashMtx.Unlock()

		case <-stop:
			return
		}
	}
}
