package exec

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
	"github.com/juju/errors"
)

type Collector struct {
	params CollectorParams

	teardown chan struct{}
	torndown chan struct{}
}

var _ collectors.Collector = &Collector{}

type CollectorParams struct {
	Common collectors.Params

	Config Config
}

func NewCollector(params CollectorParams) (*Collector, error) {
	c := &Collector{
		params:   params,
		teardown: make(chan struct{}),
		torndown: make(chan struct{}),
	}

	// TODO: validate all conds

	if c.params.Config.PollFreq == 0 {
		c.params.Config.PollFreq = 1 * time.Minute
	}

	if c.params.Config.PollFreqWhenFailed == 0 {
		c.params.Config.PollFreqWhenFailed = 5 * time.Second
	}

	go c.run()

	fmt.Printf("Collecting data using exec (%s), polling every %s\n", params.Common.ID, c.params.Config.PollFreq)

	return c, nil
}

func (c *Collector) Close() {
	close(c.teardown)
	<-c.torndown
}

func (c *Collector) getItemKey(key string) salmon.ItemKey {
	return salmon.ItemKey(c.params.Common.ID + "." + key)
}

func (c *Collector) run() {
	defer close(c.torndown)

	tickerNormal := time.NewTicker(c.params.Config.PollFreq)
	tickerWhenFailed := time.NewTicker(c.params.Config.PollFreqWhenFailed)

	var lastItemResult *salmon.Item

	runAndHandle := func() {
		itemResult := c.runCommand()

		c.params.Common.UpdatesChan <- &collectors.Update{
			Items: map[salmon.ItemKey]*salmon.Item{
				itemResult.Key: itemResult,
			},
		}

		lastItemResult = itemResult
	}

	// Run right away, without waiting for the ticker.
	runAndHandle()

	for {

		ticker := tickerNormal
		if lastItemResult.State != salmon.ItemStateOK {
			ticker = tickerWhenFailed
		}

		select {
		case <-ticker.C:
			runAndHandle()

		case <-c.teardown:
			return
		}
	}
}

func (c *Collector) runCommand() *salmon.Item {
	ret := &salmon.Item{
		Key: c.getItemKey("exec_result"),

		// State and Comment will be populated below
		Comment: c.params.Config.Comment,
	}

	cmd := exec.Command(c.params.Config.Command[0], c.params.Config.Command[1:]...)
	if err := cmd.Start(); err != nil {
		ret.State = salmon.ItemStateError
		ret.Comment += ": " + errors.Annotatef(err, "failed to start command").Error()
		return ret
	}

	// TODO: run it in a separate goroutine and wait for a timeout here (and
	// timeout should come from the config).
	//
	// Otherwise, if the command gets stuck forever for w/e reason, we won't
	// get any notification about it.
	err := cmd.Wait()
	if err != nil {
		// If the command ran normally but just returned a non-zero status code,
		// we don't handle it as an error here (since status codes are being handled
		// accordingly to the config)
		if _, ok := err.(*exec.ExitError); ok {
			err = nil
		} else {
			// Apparently the error is something else, like IO issues; so file it
			// as an error.
			ret.State = salmon.ItemStateError
			ret.Comment += ": " + errors.Annotatef(err, "failed to run command").Error()
			return ret
		}
	}

	exitCodeStr := strconv.Itoa(cmd.ProcessState.ExitCode())

	for i, cond := range c.params.Config.Conds {
		if cond.ExitCode != "" && cond.ExitCode != exitCodeStr {
			continue
		}

		// Found the matching condition, so use its result
		ret.State = cond.Result
		ret.Comment += ": " + fmt.Sprintf(
			"exit code: %s, applied condition #%d %+v",
			exitCodeStr, i, cond,
		)
		break
	}

	// If no condition matched, assume error
	if ret.State == "" {
		ret.State = salmon.ItemStateError
		ret.Comment += ": " + fmt.Sprintf("exit code: %s, did not find matching condition", exitCodeStr)
	}

	return ret
}
