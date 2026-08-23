package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"

	"github.com/dimonomid/salmon/internal/setup"
)

// watchApp owns the configuration and lifecycle state of one tray instance.
type watchApp struct {
	config *config
	core   *salmonWatchCore
}

// onReady initializes the tray application after systray has locked its OS
// thread.
func (app *watchApp) onReady() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err.Error())
	}

	mitemStatus := systray.AddMenuItem(trayStatusTitle(trayState{}), "")
	mitemExit := systray.AddMenuItem("Exit", "")

	notify := newDesktopNotificationSink()
	notify.Push("Hello there", "Salmon Watch started")

	loadTrayIcons()

	applyIcon(trayState{Alerting: overallStateUnknown})
	app.core, err = newSalmonWatchCore(salmonWatchCoreParams{
		Config:        app.config.WSClient,
		StatePath:     filepath.Join(homeDir, ".salmon-watch-state.json"),
		Notifications: notify,
		OnIconState: func(state trayState) {
			applyIcon(state)
			mitemStatus.SetTitle(trayStatusTitle(state))
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start Salmon Watch core: %s\n", err)
		os.Exit(1)
	}

	listener := setupWebserver(app.core.statusWebserver)
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

// watchConfigReadError adds setup guidance when the default configuration is
// missing.
func watchConfigReadError(configFilename string, err error) error {
	if configNotFound(err) && configFilename == defaultWatchConfigPath() {
		return fmt.Errorf("failed to read config from %s: %w\n\nHint: Run the following command to create the default configuration and desktop-autostart entry:\n\n    %s setup\n", configFilename, err, setup.ShellArgument(os.Args[0]))
	}
	return fmt.Errorf("failed to read config from %s: %w", configFilename, err)
}

// onExit shuts down Salmon Watch resources after the tray exits.
func (app *watchApp) onExit() {
	if app.core != nil {
		app.core.Close()
	}
	fmt.Println("Exiting")
}
