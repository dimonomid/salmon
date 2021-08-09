package systemd

import (
	"github.com/dimonomid/salmon"
)

type Config struct {
	UnitRules []ConfigUnitRule `yaml:"unitRules"`
}

type ConfigUnitRule struct {
	// Name is the unit name, e.g. "gpg-agent.service"
	Name string `yaml:"name"`

	// Type, if not empty, makes the rule to only apply to the units of this type.
	Type string `yaml:"type"`

	// Conds is a slice of all conds to check. If no cond matched, a salmon.ItemStateError
	// is assumed.
	Conds []ConfigCond `yaml:"conds"`
}

type ConfigCond struct {
	// If State isn't empty, it's required for the condition to be true.
	// Otherwise, it's ignored.
	State UnitState `yaml:"state"`

	// Result is the outcome of the condition if it's true.
	Result salmon.ItemState `yaml:"result"`
}
