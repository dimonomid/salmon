package main

import (
	"github.com/spf13/cobra"

	"github.com/dimonomid/salmon/logs"
	"github.com/dimonomid/salmon/version"
)

// newRootCommand constructs the Salmon command-line interface.
func newRootCommand() *cobra.Command {
	var configFilename string
	var logLevel string
	root := &cobra.Command{
		Use:          "salmon",
		Short:        "Monitor system health and publish its status",
		Version:      version.FullDescription("Salmon"),
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
	root.SetVersionTemplate("{{.Version}}")
	root.PersistentFlags().StringVar(&configFilename, "config", defaultSalmonConfig, "Config filename")
	root.Flags().StringVar(&logLevel, "log-level", "info", "Minimum log level (debug, info, warning, or error)")

	setupCommand := &cobra.Command{
		Use:   "setup",
		Short: "Perform the complete setup",
		Long:  "Perform the complete setup by creating the default configuration and service account, then installing the systemd service. Run a setup subcommand to perform only one of these operations.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := initializeSalmonConfig(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			if err := createSalmonUser(cmd.OutOrStdout()); err != nil {
				return err
			}
			if err := installSalmonService(cmd.OutOrStdout(), configFilename); err != nil {
				return err
			}
			return printSalmonStartHint(cmd.OutOrStdout())
		},
	}
	setupCommand.AddCommand(
		&cobra.Command{
			Use:   "create-config",
			Short: "Create the default configuration if it does not exist",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return initializeSalmonConfig(cmd.OutOrStdout(), configFilename)
			},
		},
		&cobra.Command{
			Use:   "create-user",
			Short: "Create the system user and group used by the Salmon service",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return createSalmonUser(cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "install-service",
			Short: "Install and enable the systemd service",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return installSalmonService(cmd.OutOrStdout(), configFilename)
			},
		},
	)

	root.AddCommand(setupCommand)
	return root
}
