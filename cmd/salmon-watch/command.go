package main

import (
	"runtime"

	"github.com/benbjohnson/clock"
	"github.com/spf13/cobra"

	"github.com/dimonomid/salmon/logs"
	"github.com/dimonomid/salmon/version"
)

// newWatchRootCommand constructs the Salmon Watch command-line interface.
func newWatchRootCommand() *cobra.Command {
	var configFilename string
	var logLevel string
	var bearerTokenOutputFilename string
	root := &cobra.Command{
		Use:          "salmon-watch",
		Short:        "Show Salmon status in the desktop tray",
		Version:      version.FullDescription("Salmon Watch"),
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			minLogLevel, err := logs.ParseLogLevel(logLevel)
			if err != nil {
				return err
			}
			cfg, err := loadConfig(configFilename)
			if err != nil {
				return watchConfigReadError(configFilename, err)
			}
			clk := clock.New()
			logger := logs.NewLogger(logs.LoggerParams{
				Clock: clk,
				Sinks: []logs.LoggerSinkParams{{MinLevel: minLogLevel}},
			}).WithNamespaceAppended("SalmonWatch")
			app := &watchApp{config: cfg, clock: clk, logger: logger}
			app.run()
			return nil
		},
	}
	root.SetVersionTemplate("{{.Version}}")
	root.PersistentFlags().StringVar(&configFilename, "config", defaultWatchConfigPath(), "Config filename")
	root.Flags().StringVar(&logLevel, "log-level", "info", "Minimum log level (debug, info, warning, or error)")

	setupCommand := &cobra.Command{
		Use:               "setup",
		Short:             "Perform the complete setup",
		Long:              "Perform the complete setup by creating the default configuration and installing the desktop autostart entry. Run a setup subcommand to perform only one of these operations.",
		Args:              cobra.NoArgs,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error { return validateWatchSetupPlatform(runtime.GOOS) },
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
	setupCommand.AddCommand(
		&cobra.Command{
			Use:   "create-config",
			Short: "Create the default configuration if it does not exist",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return initializeWatchConfig(cmd.OutOrStdout(), configFilename)
			},
		},
		&cobra.Command{
			Use:   "install-autostart",
			Short: "Install the desktop autostart entry",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return installWatchAutostart(cmd.OutOrStdout(), configFilename)
			},
		},
	)

	generateBearerTokenCommand := &cobra.Command{
		Use:   "generate-bearer-token SERVER_ID",
		Short: "Generate a bearer token for one Salmon server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateBearerToken(cmd.OutOrStdout(), configFilename, args[0], bearerTokenOutputFilename)
		},
	}
	generateBearerTokenCommand.Flags().StringVar(
		&bearerTokenOutputFilename,
		"output",
		"",
		"Token filename (defaults to tokens/SERVER_ID.token next to the salmon-watch configuration)",
	)

	root.AddCommand(setupCommand, generateBearerTokenCommand)
	return root
}
