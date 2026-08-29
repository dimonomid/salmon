package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/benbjohnson/clock"
	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"

	"github.com/dimonomid/salmon/internal/setup"
	"github.com/dimonomid/salmon/logs"
)

// watchApp owns the configuration and lifecycle state of one tray instance.
type watchApp struct {
	config       *config
	clock        clock.Clock
	logger       *logs.Logger
	core         *salmonWatchCore
	statusServer *localStatusServer
}

// run starts the tray and turns SIGINT or SIGTERM into a normal tray exit, so
// onExit gets a chance to tear down every owned resource.
func (app *watchApp) run() {
	terminationSignals := make(chan os.Signal, 1)
	signal.Notify(terminationSignals, syscall.SIGINT, syscall.SIGTERM)
	stopSignalHandler := make(chan struct{})
	go waitForWatchTerminationSignal(terminationSignals, stopSignalHandler, func(sig os.Signal) {
		// Restore the default handling before teardown starts, so a second signal
		// can still terminate the process if graceful shutdown gets stuck.
		signal.Stop(terminationSignals)
		app.logger.Log(logs.Info, "Received %s; shutting down", sig)
		systray.Quit()
	})

	// systray.Run must be the only operation that runs the tray app: it locks
	// its OS thread before invoking onReady.
	systray.Run(app.onReady, app.onExit)

	signal.Stop(terminationSignals)
	close(stopSignalHandler)
}

// waitForWatchTerminationSignal invokes onSignal for the first termination
// signal, or returns when the tray has already exited normally.
func waitForWatchTerminationSignal(
	signals <-chan os.Signal,
	stop <-chan struct{},
	onSignal func(os.Signal),
) {
	select {
	case sig := <-signals:
		onSignal(sig)
	case <-stop:
	}
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
		Clock:         app.clock,
		Logger:        app.logger,
		OnIconState: func(state trayState) {
			applyIcon(state)
			mitemStatus.SetTitle(trayStatusTitle(state))
		},
	})
	if err != nil {
		app.logger.Log(logs.Error, "Failed to start: %s", err)
		os.Exit(1)
	}

	app.statusServer = setupWebserver(app.core.statusWebserver)
	port := app.statusServer.Addr().(*net.TCPAddr).Port

	app.logger.Log(logs.Info, "Status UI is available at http://localhost:%d/status", port)

	go func() {
		if err := app.statusServer.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.logger.Log(logs.Error, "Status webserver stopped unexpectedly: %s", err)
			systray.Quit()
		}
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
	app.logger.Log(logs.Info, "Shutting down")
	if app.statusServer != nil {
		if err := app.statusServer.Close(); err != nil {
			app.logger.Log(logs.Error, "Failed to close status webserver: %s", err)
		}
	}
	if app.core != nil {
		app.core.Close()
	}
	app.logger.Log(logs.Info, "Shutdown complete")
}
