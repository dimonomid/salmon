package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dimonomid/salmon/internal/setup"
)

// validateWatchSetupPlatform prevents Linux desktop-autostart setup from
// creating unusable files on platforms that do not support it yet.
func validateWatchSetupPlatform(goos string) error {
	if goos != "linux" {
		return fmt.Errorf("salmon-watch setup is not implemented on this platform (%s)", goos)
	}
	return nil
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

// printWatchStartHint explains how to start Salmon Watch now.
func printWatchStartHint(output io.Writer, configFilename string) error {
	startCommand, err := watchStartCommand(configFilename)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "\nSalmon Watch is configured and installed for desktop autostart. To start it now, run:\n\n    %s\n", startCommand)
	return err
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
		return setup.ShellArgument(os.Args[0]), nil
	}
	absoluteConfigFilename, err := filepath.Abs(configFilename)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	return fmt.Sprintf("%s --config %s", setup.ShellArgument(os.Args[0]), setup.ShellArgument(absoluteConfigFilename)), nil
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
