package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/pflag"
)

var notify notificator

// core owns AquaScope's background Salmon connections for shutdown cleanup.
var core *aquascopeCore

func main() {
	// NOTE: Run should be the only thing which executes in main; otherwise
	// the magic done by runtime.LockOSThread() (which is called from
	// systray.Run) might not work.
	systray.Run(onReady, onExit)
}

func onReady() {
	usr, err := user.Current()
	if err != nil {
		panic(err.Error())
	}
	fmt.Println(usr.HomeDir)

	configFilename := pflag.String(
		"config", fmt.Sprintf("%s/.config/aquascope/aquascope.yml", usr.HomeDir), "Config filename",
	)

	pflag.Parse()

	cfg, err := loadConfig(*configFilename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read config from %s: %s\n", *configFilename, err)
		os.Exit(1)
	}

	mitemStatus := systray.AddMenuItem(trayStatusTitle(trayState{}), "")
	mitemExit := systray.AddMenuItem("Exit", "")

	notify = newDesktopNotificationSink()
	notify.Push("Hello there", "Aquascope started")

	loadTrayIcons()

	applyIcon(trayState{Alerting: overallStateUnknown})
	core, err = newAquascopeCore(aquascopeCoreParams{
		Config:        cfg.WSClient,
		StatePath:     filepath.Join(usr.HomeDir, ".aquascope_state.json"),
		Notifications: notify,
		OnIconState: func(state trayState) {
			applyIcon(state)
			mitemStatus.SetTitle(trayStatusTitle(state))
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start AquaScope core: %s\n", err)
		os.Exit(1)
	}

	listener := setupWebserver(core.statusWebserver)
	port := listener.Addr().(*net.TCPAddr).Port

	fmt.Printf("Listening on %d\n", port)

	go func() {
		panic(http.Serve(listener, nil))
	}()

	go func() {
		for {
			select {
			case <-mitemStatus.ClickedCh:
				open.Run(fmt.Sprintf("http://localhost:%d/status", port))

			case <-mitemExit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	if core != nil {
		core.Close()
	}
	fmt.Println("Exiting")
}
