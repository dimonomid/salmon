package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon/backend/core"
)

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
