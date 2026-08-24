package exec

import (
	"time"

	"github.com/dimonomid/salmon"
)

type Config struct {
	// Description explains what the check verifies. It prefixes the dynamic
	// execution details published with the item.
	Description string `yaml:"description"`

	// Command is a slice of strings: first one is the command, all others will be
	// arguments. Example: []string{"bash", "-c", "(( $(df --output=avail / | sed 1d) > 1000000 ))"}
	Command []string `yaml:"command"`

	// PollInterval is how often to run the command. Default: 1 minute.
	PollInterval time.Duration `yaml:"pollInterval"`

	PollIntervalWhenUnhealthy time.Duration `yaml:"pollIntervalWhenUnhealthy"`

	// Conds is a slice of all conds to check. If no cond matched, a salmon.ItemStateError
	// is assumed.
	Conds []ConfigCond `yaml:"conds"`
}

type ConfigCond struct {
	// If ExitCode isn't an empty string, it's interpreted as an int, and the process
	// must exit with that code for the condition to be true.
	// Otherwise, it's ignored.
	//
	// It's a string and not an int just to make the zero value to be
	// distinguishable from 0, which is a valid exit code.
	ExitCode string `yaml:"exitCode"`

	// Result is the outcome of the condition if it's true.
	Result salmon.ItemState `yaml:"result"`
}
