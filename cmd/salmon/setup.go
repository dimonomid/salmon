package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dimonomid/salmon/internal/setup"
)

const (
	defaultSalmonConfig = "/etc/salmon.yml"
	salmonUnitPath      = "/etc/systemd/system/salmon.service"
)

// initializeSalmonConfig creates the configuration when absent and reports the
// result to output.
func initializeSalmonConfig(output io.Writer, configFilename string) error {
	created, err := setup.EnsureFile(configFilename, string(mustSetupAsset("assets/setup/salmon.yml")))
	if err != nil {
		return err
	}
	return setup.ReportEnsureResult(output, "configuration", configFilename, created)
}

// installSalmonService creates the systemd unit when absent, enables it, and
// reports the result to output.
func installSalmonService(output io.Writer, configFilename string) error {
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
	unit, err := setup.RenderSystemdUnitTemplate("salmon.service.tpl", string(mustSetupAsset("assets/setup/salmon.service.tpl")), struct {
		Executable     string
		ConfigFilename string
	}{executable, absoluteConfigFilename})
	if err != nil {
		return err
	}
	created, err := setup.InstallSystemdService(salmonUnitPath, "salmon.service", unit, runCommand)
	if err != nil {
		return err
	}
	return setup.ReportEnsureResult(output, "systemd service", salmonUnitPath, created)
}

// printSalmonStartHint explains how to start the configured service now.
func printSalmonStartHint(output io.Writer) error {
	_, err := fmt.Fprint(output, "\nService is configured and installed. To start it, run:\n\n    sudo systemctl start salmon.service\n")
	return err
}

// runCommand executes a command, forwarding its standard output and error to
// Salmon's own streams.
func runCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
