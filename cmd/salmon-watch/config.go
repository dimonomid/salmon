package main

import (
	"io/ioutil"

	"github.com/dimonomid/salmon/wsclient"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

type config struct {
	WSClient wsclient.Config `yaml:"wsClient"`
}

func loadConfig(filename string) (*config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cfg config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Trace(err)
	}

	return &cfg, nil
}
