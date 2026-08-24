package exec

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
	"github.com/juju/errors"
)

type Collector struct {
	params CollectorParams

	teardown  chan struct{}
	torndown  chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

var _ collectors.Collector = &Collector{}

type CollectorParams struct {
	Common collectors.Params

	Config Config
}

func NewCollector(params CollectorParams) (*Collector, error) {
	if err := validateConfig(params.Config); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Collector{
		params:   params,
		teardown: make(chan struct{}),
		torndown: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	if c.params.Config.PollInterval == 0 {
		c.params.Config.PollInterval = 1 * time.Minute
	}

	if c.params.Config.PollIntervalWhenUnhealthy == 0 {
		c.params.Config.PollIntervalWhenUnhealthy = 5 * time.Second
	}

	go c.run()

	fmt.Printf("Collecting data using exec (%s), polling every %s\n", params.Common.ID, c.params.Config.PollInterval)

	return c, nil
}

func validateConfig(config Config) error {
	if len(config.Command) == 0 || config.Command[0] == "" {
		return fmt.Errorf("command must not be empty")
	}
	if config.PollInterval < 0 {
		return fmt.Errorf("pollInterval must not be negative")
	}
	if config.PollIntervalWhenUnhealthy < 0 {
		return fmt.Errorf("pollIntervalWhenUnhealthy must not be negative")
	}
	for i, condition := range config.Conditions {
		if condition.ExitCode != "" {
			if _, err := strconv.Atoi(condition.ExitCode); err != nil {
				return fmt.Errorf("condition #%d has invalid exitCode %q", i, condition.ExitCode)
			}
		}
		if !salmon.IsItemStateValid(condition.Result) {
			return fmt.Errorf("condition #%d has invalid result %q", i, condition.Result)
		}
	}
	return nil
}

func (c *Collector) Close() {
	c.closeOnce.Do(func() {
		c.cancel()
		close(c.teardown)
	})
	<-c.torndown
}

func (c *Collector) getItemKey(key string) salmon.ItemKey {
	return salmon.ItemKey(c.params.Common.ID + "." + key)
}

func (c *Collector) run() {
	defer close(c.torndown)

	tickerNormal := time.NewTicker(c.params.Config.PollInterval)
	tickerWhenUnhealthy := time.NewTicker(c.params.Config.PollIntervalWhenUnhealthy)
	defer tickerNormal.Stop()
	defer tickerWhenUnhealthy.Stop()

	var lastItemResult *salmon.Item

	runAndHandle := func() bool {
		itemResult := c.runCommand()

		update := &collectors.Update{
			Items: map[salmon.ItemKey]*salmon.Item{
				itemResult.Key: itemResult,
			},
		}
		select {
		case c.params.Common.UpdatesChan <- update:
		case <-c.teardown:
			return false
		}

		lastItemResult = itemResult
		return true
	}

	// Run right away, without waiting for the ticker.
	if !runAndHandle() {
		return
	}

	for {

		ticker := tickerNormal
		if lastItemResult.State != salmon.ItemStateOK {
			ticker = tickerWhenUnhealthy
		}

		select {
		case <-ticker.C:
			if !runAndHandle() {
				return
			}

		case <-c.teardown:
			return
		}
	}
}

func (c *Collector) runCommand() *salmon.Item {
	ret := &salmon.Item{
		Key: c.getItemKey("exec_result"),

		// State will be populated below. Details starts with the configured
		// description and gains the dynamic execution result.
		Details: c.params.Config.Description,
	}

	cmd := exec.CommandContext(c.ctx, c.params.Config.Command[0], c.params.Config.Command[1:]...)
	if err := cmd.Start(); err != nil {
		ret.State = salmon.ItemStateError
		ret.Details = appendDetails(ret.Details, errors.Annotatef(err, "failed to start command").Error())
		return ret
	}

	// TODO: add a configurable command timeout. Close cancels an active command,
	// but during normal operation a stuck command prevents this collector from
	// producing further updates or reporting that the check itself is stuck.
	err := cmd.Wait()
	if err != nil {
		if c.ctx.Err() != nil {
			ret.State = salmon.ItemStateError
			ret.Details = appendDetails(ret.Details, "command was canceled: "+c.ctx.Err().Error())
			return ret
		}
		// If the command ran normally but just returned a non-zero status code,
		// we don't handle it as an error here (since status codes are being handled
		// accordingly to the config)
		if _, ok := err.(*exec.ExitError); ok {
			err = nil
		} else {
			// Apparently the error is something else, like IO issues; so file it
			// as an error.
			ret.State = salmon.ItemStateError
			ret.Details = appendDetails(ret.Details, errors.Annotatef(err, "failed to run command").Error())
			return ret
		}
	}

	exitCodeStr := strconv.Itoa(cmd.ProcessState.ExitCode())

	for i, cond := range c.params.Config.Conditions {
		if cond.ExitCode != "" && cond.ExitCode != exitCodeStr {
			continue
		}

		// Found the matching condition, so use its result
		ret.State = cond.Result
		ret.Details = appendDetails(ret.Details, fmt.Sprintf(
			"exit code: %s, applied condition #%d %+v",
			exitCodeStr, i, cond,
		))
		break
	}

	// If no condition matched, assume error
	if ret.State == "" {
		ret.State = salmon.ItemStateError
		ret.Details = appendDetails(ret.Details, fmt.Sprintf("exit code: %s, did not find matching condition", exitCodeStr))
	}

	return ret
}

func appendDetails(existing, additional string) string {
	if existing == "" {
		return additional
	}
	return existing + ": " + additional
}
