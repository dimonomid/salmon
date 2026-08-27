package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
	"github.com/juju/errors"
)

// maxOutputLineBytes limits how much of a command's first stdout line is kept
// for incident details. Longer lines are truncated and marked with an ellipsis.
const maxOutputLineBytes = 200

// firstLineWriter retains up to maxOutputLineBytes from the first line. It
// continues to accept all subsequent output but discards it, effectively acting
// like /dev/null after the retained text. Draining the output prevents a
// verbose command from blocking, while discarding it keeps memory usage bounded.
type firstLineWriter struct {
	line      []byte
	complete  bool
	truncated bool
}

func (w *firstLineWriter) Write(p []byte) (int, error) {
	written := len(p)
	if w.complete {
		return written, nil
	}
	if newline := bytes.IndexByte(p, '\n'); newline >= 0 {
		p = p[:newline]
		w.complete = true
	}
	remaining := maxOutputLineBytes - len(w.line)
	if len(p) > remaining {
		p = p[:remaining]
		w.complete = true
		w.truncated = true
	}
	w.line = append(w.line, p...)
	return written, nil
}

func (w *firstLineWriter) String() string {
	line := strings.TrimSpace(string(w.line))
	if line != "" && w.truncated {
		line += "…"
	}
	return line
}

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
	if params.Config.Conditions == nil {
		params.Config.Conditions = []ConfigCondition{
			{ExitCode: "0", Result: salmon.ItemStateOK},
			{Result: salmon.ItemStateError},
		}
	}
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
	if len(config.Conditions) == 0 {
		return fmt.Errorf("conditions must not be empty")
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
	var stdout firstLineWriter
	// Capture stdout through a pipe instead of cmd.Output or assigning stdout
	// directly to firstLineWriter. The pipe lets us drain output concurrently,
	// so a verbose command cannot block when the OS pipe buffer fills, without
	// buffering all of its output in memory.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		ret.State = salmon.ItemStateError
		ret.Details = appendDetails(ret.Details, errors.Annotatef(err, "failed to capture command output").Error())
		return ret
	}
	if err := cmd.Start(); err != nil {
		ret.State = salmon.ItemStateError
		ret.Details = appendDetails(ret.Details, errors.Annotatef(err, "failed to start command").Error())
		return ret
	}
	stdoutDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stdout, stdoutPipe)
		close(stdoutDone)
	}()

	// StdoutPipe must be fully drained before Wait: Wait closes the pipe and can
	// otherwise discard output that the reader has not consumed yet. On
	// cancellation we close it ourselves, because a descendant may have inherited
	// stdout and could keep the reader waiting for EOF after the command is killed.
	//
	// TODO: add a configurable command timeout. Close cancels an active command,
	// but during normal operation a stuck command prevents this collector from
	// producing further updates or reporting that the check itself is stuck.
	select {
	case <-stdoutDone:
	case <-c.ctx.Done():
		_ = stdoutPipe.Close()
		<-stdoutDone
	}
	err = cmd.Wait()
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
		fmt.Printf("Exec result (%s): exit code %s, applied condition #%d %+v\n",
			c.params.Common.ID, exitCodeStr, i, cond)
		break
	}

	// If no condition matched, assume error
	if ret.State == "" {
		ret.State = salmon.ItemStateError
		fmt.Printf("Exec result (%s): exit code %s, did not find matching condition\n",
			c.params.Common.ID, exitCodeStr)
	}

	dynamicDetails := stdout.String()
	if dynamicDetails == "" {
		dynamicDetails = "exit code: " + exitCodeStr
	}
	ret.Details = appendDetails(ret.Details, dynamicDetails)

	return ret
}

func appendDetails(existing, additional string) string {
	if existing == "" {
		return additional
	}
	return existing + ": " + additional
}
