package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/0xAX/notificator"
	"github.com/getlantern/systray"

	"github.com/dimonomid/salmon"
)

var (
	iconSalmonGray    []byte
	iconSalmonGreen   []byte
	iconSalmonMagenta []byte
	iconSalmonYellow  []byte
	iconSalmonRed     []byte
	iconTransparent   []byte

	iconFlashMtx  sync.Mutex
	iconFlashStop chan struct{}
)

type overallState int

const (
	overallStateUnknown overallState = iota
	overallStateOK
	overallStateInternalError
	overallStateWarning
	overallStateError
)

func loadTrayIcons() {
	iconSalmonGray = MustAsset("assets/salmon_gray.png")
	iconSalmonGreen = MustAsset("assets/salmon_green.png")
	iconSalmonMagenta = MustAsset("assets/salmon_magenta.png")
	iconSalmonYellow = MustAsset("assets/salmon_yellow.png")
	iconSalmonRed = MustAsset("assets/salmon_red.png")
	iconTransparent = MustAsset("assets/salmon_transparent.png")
}

func getOverallStateFromItems(items []*salmon.ItemWContext) overallState {
	ret := overallStateOK

	for _, item := range items {
		cur := getOverallStateFromItem(item)
		if ret < cur {
			ret = cur
		}
	}

	return ret
}

func getOverallStateFromItem(item *salmon.ItemWContext) overallState {
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

func sendNotification(title, text string) {
	notify.Push(title, text, "", notificator.UR_NORMAL)
}

func applyIcon(state overallState) {
	var icon []byte

	switch state {
	case overallStateUnknown:
		icon = iconSalmonGray
	case overallStateOK:
		icon = iconSalmonGreen
	case overallStateInternalError:
		icon = iconSalmonMagenta
	case overallStateWarning:
		icon = iconSalmonYellow
	case overallStateError:
		icon = iconSalmonRed
	default:
		panic(fmt.Sprintf("invalid state %d", state))
	}

	iconFlashMtx.Lock()
	if iconFlashStop != nil {
		close(iconFlashStop)
		iconFlashStop = nil
	}

	if state == overallStateOK {
		systray.SetIcon(icon)
		iconFlashMtx.Unlock()
		return
	}

	stop := make(chan struct{})
	iconFlashStop = stop
	systray.SetIcon(icon)
	iconFlashMtx.Unlock()

	go flashIcon(icon, stop)
}

// flashIcon alternates a non-OK icon with a transparent icon until its state
// is replaced by another call to applyIcon.
func flashIcon(icon []byte, stop <-chan struct{}) {
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
				systray.SetIcon(iconTransparent)
			}
			iconFlashMtx.Unlock()

		case <-stop:
			return
		}
	}
}
