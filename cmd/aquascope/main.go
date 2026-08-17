package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"github.com/benbjohnson/clock"
	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/pflag"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/wsclient"
)

var notify notificator

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
		fmt.Fprintf(os.Stderr, "failed to read config from %s: %s\n", configFilename, err)
		os.Exit(1)
	}

	mitemStatus := systray.AddMenuItem("Status", "")
	mitemExit := systray.AddMenuItem("Exit", "")

	notify = newDesktopNotificationSink()
	notify.Push("Hello there", "Aquascope started")

	loadTrayIcons()

	applyIcon(overallStateUnknown)
	incidentState, err := newIncidentState(filepath.Join(usr.HomeDir, ".aquascope_state.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load AquaScope state: %s\n", err)
		os.Exit(1)
	}
	statusWebserver := newStatusWebserver(statusWebserverParams{
		OnSnooze:   incidentState.Snooze,
		OnUnsnooze: incidentState.Unsnooze,
	})
	incidentState.OnUpdate = func(snapshot incidentSnapshot) {
		statusWebserver.SetOngoingIncidents(snapshot)
		applyIcon(getOverallStateFromItems(snapshot.Alerting))
	}

	c, err := wsclient.NewCombiner(wsclient.CombinerParams{
		Config: cfg.WSClient,

		OngoingIncidentsHandler: func(notif *salmon.Notification) {
			snapshot := incidentState.Update(notif.OngoingIncidents.Total)

			d, _ := json.MarshalIndent(notif, "", "  ")
			fmt.Println(string(d))

			for _, item := range notif.OngoingIncidents.Added {
				if incidentState.IsSnoozed(string(item.Key)) {
					continue
				}
				notify.Push(string(item.State)+": "+string(item.Key), item.Comment)
			}

			// Do not show desktop notifications for updates to existing incidents:
			// volatile details (such as connection ports in an error) can change
			// repeatedly while the underlying incident is still the same.

			for _, item := range notif.OngoingIncidents.Removed {
				if incidentState.IsSnoozed(string(item.Key)) {
					continue
				}
				notify.Push("OK: "+string(item.Key), "")
			}

			state := getOverallStateFromItems(snapshot.Alerting)
			fmt.Println("Overall state:", state)
		},

		Clock: clock.New(),
	})
	if err != nil {
		fmt.Println("Error creating client:", err)
		os.Exit(1)
	}

	_ = c

	listener := setupWebserver(statusWebserver)
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
	fmt.Println("Exiting")
}
