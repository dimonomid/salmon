package main

import (
	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon/logs"
)

var watchTestLogger = logs.NewLogger(logs.LoggerParams{Clock: clock.New()})
