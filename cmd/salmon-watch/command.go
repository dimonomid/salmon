package main

import (
	"github.com/getlantern/systray"
	"github.com/spf13/cobra"
)

// newWatchRootCommand constructs the Salmon Watch command-line interface.
func newWatchRootCommand() *cobra.Command {
	var configFilename string
	root := &cobra.Command{
		Use:          "salmon-watch",
		Short:        "Show Salmon status in the desktop tray",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadConfig(configFilename)
			if err != nil {
				return watchConfigReadError(configFilename, err)
			}
			app := &watchApp{config: cfg}
			// systray.Run must be the only operation that runs the tray app: it
			// locks its OS thread before invoking onReady.
			systray.Run(app.onReady, app.onExit)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&configFilename, "config", defaultWatchConfigPath(), "Config filename")

	configCommand := &cobra.Command{Use: "config", Short: "Manage configuration"}
	configCommand.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create the default configuration if it does not exist",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initializeWatchConfig(cmd.OutOrStdout(), configFilename)
		},
	})

	autostartCommand := &cobra.Command{Use: "autostart", Short: "Manage desktop autostart"}
	autostartCommand.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install the desktop autostart entry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return installWatchAutostart(cmd.OutOrStdout(), configFilename)
		},
	})

	setupCommand := &cobra.Command{
		Use:   "setup",
		Short: "Perform the complete setup",
		Long:  "Equivalent to running `salmon-watch config init` followed by `salmon-watch autostart install`.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := initializeWatchConfig(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			if err := installWatchAutostart(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			return printWatchStartHint(cmd.OutOrStdout(), configFilename)
		},
	}

	root.AddCommand(configCommand, autostartCommand, setupCommand)
	return root
}
