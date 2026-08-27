package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon/backend/core"
	"github.com/dimonomid/salmon/internal/setup"
	"github.com/dimonomid/salmon/logs"
)

// runSalmon loads the configuration and runs the monitoring core until a
// termination signal arrives.
func runSalmon(configFilename string, minLogLevel logs.LogLevel) error {
	cfg, err := loadConfig(configFilename)
	if err != nil {
		return salmonConfigReadError(configFilename, err)
	}

	clk := clock.New()
	logger := logs.NewLogger(logs.LoggerParams{
		Clock: clk,
		Sinks: []logs.LoggerSinkParams{{MinLevel: minLogLevel}},
	}).WithNamespaceAppended("Salmon")
	c, err := core.NewCore(cfg.Core, core.Params{Clock: clk, Logger: logger})
	if err != nil {
		return fmt.Errorf("failed to initialize salmon core: %w", err)
	}

	logger.Log(logs.Info, "Monitoring started")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	logger.Log(logs.Info, "Received %s; shutting down", sig)
	c.Close()
	logger.Log(logs.Info, "Shutdown complete")
	return nil
}

// salmonConfigReadError adds setup guidance when the default configuration is
// missing.
func salmonConfigReadError(configFilename string, err error) error {
	if configNotFound(err) && configFilename == defaultSalmonConfig {
		return fmt.Errorf("failed to read config from %s: %w\n\nHint: Run the following command to create the default configuration and install the service:\n\n    sudo %s setup\n", configFilename, err, setup.ShellArgument(os.Args[0]))
	}
	return fmt.Errorf("failed to read config from %s: %w", configFilename, err)
}
