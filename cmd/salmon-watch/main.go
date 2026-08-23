package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/dimonomid/salmon/internal/setup"
)

var notify notificator

// core owns Salmon Watch's background Salmon connections for shutdown cleanup.
var core *salmonWatchCore

var watchConfig *config

// main executes the Salmon Watch command-line application.
func main() {
	if err := newWatchRootCommand().Execute(); err != nil {
		// Cobra already printed the error message.
		os.Exit(1)
	}
}

// newWatchRootCommand constructs the Salmon Watch command-line interface.
func newWatchRootCommand() *cobra.Command {
	var configFilename string
	root := &cobra.Command{
		Use:          "salmon-watch",
		Short:        "Show Salmon status in the desktop tray",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadConfig(configFilename)
			if err != nil {
				return fmt.Errorf("%s", watchConfigReadError(configFilename, err))
			}
			watchConfig = cfg
			// systray.Run must be the only operation that runs the tray app: it
			// locks its OS thread before invoking onReady.
			systray.Run(onReady, onExit)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&configFilename, "config", defaultWatchConfigPath(), "Config filename")

	configCommand := &cobra.Command{Use: "config", Short: "Manage configuration"}
	configCommand.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create the default configuration if it does not exist",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initializeWatchConfig(cmd.OutOrStdout(), configFilename)
		},
	})

	autostartCommand := &cobra.Command{Use: "autostart", Short: "Manage desktop autostart"}
	autostartCommand.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install the desktop autostart entry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return installWatchAutostart(cmd.OutOrStdout(), configFilename)
		},
	})

	setupCommand := &cobra.Command{
		Use:   "setup",
		Short: "Perform the complete setup",
		Long:  "Equivalent to running `salmon-watch config init` followed by `salmon-watch autostart install`.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := initializeWatchConfig(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			if err := installWatchAutostart(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			startCommand, err := watchStartCommand(configFilename)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nSalmon Watch is configured and installed for desktop autostart. To start it now, run:\n\n    %s\n", startCommand)
			return err
		},
	}

	root.AddCommand(configCommand, autostartCommand, setupCommand)
	return root
}

// initializeWatchConfig creates the configuration when absent and reports the
// result to output.
func initializeWatchConfig(output io.Writer, configFilename string) error {
	created, err := setup.EnsureFile(configFilename, string(mustEmbeddedAsset("assets/setup/salmon-watch.yml")))
	if err != nil {
		return err
	}
	return setup.ReportEnsureResult(output, "configuration", configFilename, created)
}

// installWatchAutostart creates the desktop autostart entry when absent and
// reports the result to output.
func installWatchAutostart(output io.Writer, configFilename string) error {
	absoluteConfigFilename, err := filepath.Abs(configFilename)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if _, err := loadConfig(absoluteConfigFilename); err != nil {
		return fmt.Errorf("validate config at %s: %w", absoluteConfigFilename, err)
	}
	executable, err := setup.ExecutablePath()
	if err != nil {
		return err
	}
	entry, err := setup.RenderDesktopEntryTemplate("salmon-watch.desktop.tpl", string(mustEmbeddedAsset("assets/setup/salmon-watch.desktop.tpl")), struct {
		Executable     string
		ConfigFilename string
	}{executable, absoluteConfigFilename})
	if err != nil {
		return err
	}
	created, err := setup.EnsureFile(defaultWatchAutostartPath(), entry)
	if err != nil {
		return err
	}
	return setup.ReportEnsureResult(output, "desktop autostart entry", defaultWatchAutostartPath(), created)
}

// onReady initializes the tray application after systray has locked its OS
// thread.
func onReady() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err.Error())
	}

	mitemStatus := systray.AddMenuItem(trayStatusTitle(trayState{}), "")
	mitemExit := systray.AddMenuItem("Exit", "")

	notify = newDesktopNotificationSink()
	notify.Push("Hello there", "Salmon Watch started")

	loadTrayIcons()

	applyIcon(trayState{Alerting: overallStateUnknown})
	core, err = newSalmonWatchCore(salmonWatchCoreParams{
		Config:        watchConfig.WSClient,
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

// watchConfigReadError adds setup guidance when the default configuration is
// missing.
func watchConfigReadError(configFilename string, err error) string {
	message := fmt.Sprintf("failed to read config from %s: %s", configFilename, err)
	if configNotFound(err) && configFilename == defaultWatchConfigPath() {
		message += fmt.Sprintf("\n\nHint: Run the following command to create the default configuration and desktop-autostart entry:\n\n    %s setup\n", shellArgument(os.Args[0]))
	}
	return message
}

// defaultWatchConfigPath returns the default XDG configuration path.
func defaultWatchConfigPath() string {
	return filepath.Join(userConfigHome(), "salmon-watch", "salmon-watch.yml")
}

// defaultWatchAutostartPath returns the default XDG desktop-autostart path.
func defaultWatchAutostartPath() string {
	return filepath.Join(userConfigHome(), "autostart", "salmon-watch.desktop")
}

// watchStartCommand returns a command that starts Salmon Watch with the given
// configuration, resolving custom paths so the command works from any
// directory.
func watchStartCommand(configFilename string) (string, error) {
	if configFilename == defaultWatchConfigPath() {
		return shellArgument(os.Args[0]), nil
	}
	absoluteConfigFilename, err := filepath.Abs(configFilename)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	return fmt.Sprintf("%s --config %s", shellArgument(os.Args[0]), shellArgument(absoluteConfigFilename)), nil
}

// shellArgument formats an argument for a POSIX shell command line. It leaves
// ordinary path-like values unquoted to keep command hints easy to read.
func shellArgument(argument string) string {
	if argument != "" && strings.IndexFunc(argument, func(character rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", character)
	}) == -1 {
		return argument
	}
	return "'" + strings.ReplaceAll(argument, "'", "'\"'\"'") + "'"
}

// userConfigHome returns XDG_CONFIG_HOME or fall back to $HOME/.config
func userConfigHome() string {
	if directory := os.Getenv("XDG_CONFIG_HOME"); directory != "" {
		return directory
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".config"
	}
	return filepath.Join(homeDir, ".config")
}

// onExit shuts down Salmon Watch resources after the tray exits.
func onExit() {
	if core != nil {
		core.Close()
	}
	fmt.Println("Exiting")
}
