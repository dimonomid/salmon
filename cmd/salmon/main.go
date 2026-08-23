package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/benbjohnson/clock"
	"github.com/spf13/cobra"

	"github.com/dimonomid/salmon/backend/core"
	"github.com/dimonomid/salmon/internal/setup"
)

const (
	defaultSalmonConfig = "/etc/salmon.yml"
	salmonUnitPath      = "/etc/systemd/system/salmon.service"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		// Cobra already printed the error message.
		os.Exit(1)
	}
}

// newRootCommand constructs the Salmon command-line interface.
func newRootCommand() *cobra.Command {
	var configFilename string
	root := &cobra.Command{
		Use:          "salmon",
		Short:        "Monitor system health and publish its status",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runSalmon(configFilename)
		},
	}
	root.PersistentFlags().StringVar(&configFilename, "config", defaultSalmonConfig, "Config filename")

	configCommand := &cobra.Command{Use: "config", Short: "Manage configuration"}
	configCommand.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create the default configuration if it does not exist",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initializeSalmonConfig(cmd.OutOrStdout(), configFilename)
		},
	})

	serviceCommand := &cobra.Command{Use: "service", Short: "Manage the systemd service"}
	installCommand := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the systemd service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return installSalmonService(cmd.OutOrStdout(), configFilename)
		},
	}
	serviceCommand.AddCommand(installCommand)

	setupCommand := &cobra.Command{
		Use:   "setup",
		Short: "Perform the complete setup",
		Long:  "Equivalent to running `salmon config init` followed by `salmon service install`.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := initializeSalmonConfig(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			if err := installSalmonService(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), "\nService is configured and installed. To start it, run:\n\n    sudo systemctl start salmon.service\n")
			return err
		},
	}

	root.AddCommand(configCommand, serviceCommand, setupCommand)
	return root
}

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

// runSalmon loads the configuration and runs the monitoring core until a
// termination signal arrives.
func runSalmon(configFilename string) error {
	cfg, err := loadConfig(configFilename)
	if err != nil {
		return salmonConfigReadError(configFilename, err)
	}

	c, err := core.NewCore(cfg.Core, core.Params{Clock: clock.New()})
	if err != nil {
		return fmt.Errorf("failed to initialize salmon core: %w", err)
	}

	fmt.Println("Salmon core is initialized")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	fmt.Println("Exiting...")
	c.Close()
	fmt.Println("Bye.")
	return nil
}

// salmonConfigReadError adds setup guidance when the default configuration is
// missing.
func salmonConfigReadError(configFilename string, err error) error {
	if configNotFound(err) && configFilename == defaultSalmonConfig {
		return fmt.Errorf("failed to read config from %s: %w\n\nHint: Run the following command to create the default configuration and install the service:\n\n    sudo %s setup\n", configFilename, err, os.Args[0])
	}
	return fmt.Errorf("failed to read config from %s: %w", configFilename, err)
}

// runCommand executes a command, forwarding its standard output and error to
// Salmon's own streams.
func runCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
