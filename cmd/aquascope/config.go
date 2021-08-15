package main

import "github.com/dimonomid/salmon/wsclient"

type config struct {
	WSClient wsclient.Config `yaml:"wsClient"`
}
