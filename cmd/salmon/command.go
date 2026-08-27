package main

import (
	"github.com/spf13/cobra"

	"github.com/dimonomid/salmon/logs"
)

// newRootCommand constructs the Salmon command-line interface.
func newRootCommand() *cobra.Command {
	var configFilename string
	var logLevel string
	root := &cobra.Command{
		Use:          "salmon",
		Short:        "Monitor system health and publish its status",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			minLogLevel, err := logs.ParseLogLevel(logLevel)
			if err != nil {
				return err
			}
			return runSalmon(configFilename, minLogLevel)
		},
	}
	root.PersistentFlags().StringVar(&configFilename, "config", defaultSalmonConfig, "Config filename")
	root.Flags().StringVar(&logLevel, "log-level", "info", "Minimum log level (debug, info, warning, or error)")

	configCommand := &cobra.Command{Use: "config", Short: "Manage configuration"}
	configCommand.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create the default configuration if it does not exist",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initializeSalmonConfig(cmd.OutOrStdout(), configFilename)
		},
	})

	serviceCommand := &cobra.Command{Use: "service", Short: "Manage the systemd service"}
	installCommand := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the systemd service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return installSalmonService(cmd.OutOrStdout(), configFilename)
		},
	}
	serviceCommand.AddCommand(installCommand)

	setupCommand := &cobra.Command{
		Use:   "setup",
		Short: "Perform the complete setup",
		Long:  "Equivalent to running `salmon config init` followed by `salmon service install`.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := initializeSalmonConfig(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			if err := installSalmonService(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			return printSalmonStartHint(cmd.OutOrStdout())
		},
	}

	root.AddCommand(configCommand, serviceCommand, setupCommand)
	return root
}
