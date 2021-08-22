package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/dimonomid/salmon/backend/core"
	"github.com/juju/errors"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"

	"github.com/benbjohnson/clock"
)

func main() {
	configFilename := pflag.String(
		"config", "/etc/salmon.yml", "Config filename",
	)

	pflag.Parse()

	cfg, err := loadConfig(*configFilename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read config from %s: %s\n", configFilename, err)
		os.Exit(1)
	}

	c, err := core.NewCore(
		*cfg,
		core.Params{
			Clock: clock.New(),
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize salmon core: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Salmon core is initialized")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	<-sigCh
	fmt.Println("Exiting...")
	c.Close()

	fmt.Println("Bye.")
}

func loadConfig(filename string) (*core.Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cfg core.Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Trace(err)
	}

	return &cfg, nil
}
