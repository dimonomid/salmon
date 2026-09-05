package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

// CmdRunner runs an external command. InstallSystemdService accepts it so tests
// can verify systemctl invocations without calling a real systemd manager.
type CmdRunner func(name string, args ...string) error

// RenderSystemdUnitTemplate renders a systemd unit template using systemd-safe
// command arguments.
func RenderSystemdUnitTemplate(name, source string, data interface{}) (string, error) {
	return renderTemplate(name, source, data, template.FuncMap{
		"systemdUnitArgument": systemdUnitArgument,
	})
}

// systemdUnitArgument formats an argument for a systemd unit command line.
func systemdUnitArgument(argument string) string {
	return strings.ReplaceAll(strconv.Quote(argument), "%", "%%")
}

// InstallSystemdService creates a systemd unit when absent, then reloads
// systemd and enables it. The caller supplies CmdRunner so this behavior is
// testable without a real manager.
func InstallSystemdService(unitPath, unitName, contents string, run CmdRunner) (bool, error) {
	unitDirectory := filepath.Dir(unitPath)
	info, err := os.Stat(unitDirectory)
	if err != nil {
		return false, fmt.Errorf("access systemd unit directory %s: %w", unitDirectory, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("systemd unit directory %s is not a directory", unitDirectory)
	}

	created, err := EnsureFile(unitPath, contents)
	if err != nil {
		return false, err
	}
	if err := run("systemctl", "daemon-reload"); err != nil {
		return false, fmt.Errorf("reload systemd: %w", err)
	}

	if err := run("systemctl", "enable", unitName); err != nil {
		return false, fmt.Errorf("enable service: %w", err)
	}
	return created, nil
}
