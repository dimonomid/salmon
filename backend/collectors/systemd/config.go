package systemd

import (
	"github.com/dimonomid/salmon"
)

type Config struct {
	UnitRules []ConfigUnitRule `yaml:"unitRules"`
}

type ConfigUnitRule struct {
	// Names, when non-empty, limits the rule to these exact unit names, such as
	// "gpg-agent.service".
	Names []string `yaml:"names"`

	// Type, if not empty, makes the rule to only apply to the units of this type.
	Type string `yaml:"type"`

	// Conditions contains the conditions to check. If none matches, a
	// salmon.ItemStateError is assumed.
	Conditions []ConfigCondition `yaml:"conditions"`
}

type ConfigCondition struct {
	// If State isn't empty, it's required for the condition to be true.
	// Otherwise, it's ignored.
	State UnitState `yaml:"state"`

	// Result is the outcome of the condition if it's true.
	Result salmon.ItemState `yaml:"result"`
}
