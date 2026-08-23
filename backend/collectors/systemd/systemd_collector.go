package systemd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
)

type Collector struct {
	params CollectorParams

	pr        Provider
	teardown  chan struct{}
	torndown  chan struct{}
	closeOnce sync.Once
}

var _ collectors.Collector = &Collector{}

type CollectorParams struct {
	Common collectors.Params

	Config Config

	// ProviderFactory exists primarily so black-box tests can replace the
	// external systemd/DBus boundary with a controlled provider. Production
	// callers should normally leave it nil to use the DBus-backed provider. A
	// custom provider must obey the shutdown contract documented by Provider.
	ProviderFactory func(ProviderParams) (Provider, error)
}

func NewCollector(params CollectorParams) (*Collector, error) {
	if err := validateConfig(params.Config); err != nil {
		return nil, err
	}
	providerUpdCh := make(chan *UnitUpdate, 16)

	providerFactory := params.ProviderFactory
	if providerFactory == nil {
		providerFactory = func(providerParams ProviderParams) (Provider, error) {
			return NewProviderCoreos(ProviderCoreosParams{Common: providerParams})
		}
	}
	pr, err := providerFactory(ProviderParams{UnitUpdatesChan: providerUpdCh})
	if err != nil {
		return nil, err
	}

	c := &Collector{
		params:   params,
		pr:       pr,
		teardown: make(chan struct{}),
		torndown: make(chan struct{}),
	}

	go c.run(providerUpdCh)

	fmt.Printf("Collecting data from systemd (%s)\n", params.Common.ID)

	return c, nil
}

func validateConfig(config Config) error {
	for ruleIndex, rule := range config.UnitRules {
		for conditionIndex, condition := range rule.Conds {
			if !salmon.IsItemStateValid(condition.Result) {
				return fmt.Errorf("rule #%d condition #%d has invalid result %q", ruleIndex, conditionIndex, condition.Result)
			}
		}
	}
	return nil
}

func (c *Collector) Close() {
	c.closeOnce.Do(func() {
		// Stop forwarding updates first. The run goroutine continues draining the
		// provider channel so the provider cannot block while it shuts down.
		close(c.teardown)
		c.pr.Close()

		// Provider shutdown closes providerUpdCh. Once it has been drained, the
		// run goroutine exits and closes c.torndown.
		<-c.torndown
	})
}

func (c *Collector) run(providerUpdCh chan *UnitUpdate) {
	defer close(c.torndown)
	firstUpdate := true
	shuttingDown := false

	for sysUpd := range providerUpdCh {
		if !shuttingDown {
			select {
			case <-c.teardown:
				shuttingDown = true
			default:
			}
		}
		if shuttingDown {
			// Keep draining until provider shutdown closes providerUpdCh.
			continue
		}

		if sysUpd.Err != nil {
			if !c.sendUpdate(&collectors.Update{
				Err: fmt.Errorf("got error from systemd conn: %w", sysUpd.Err),
			}) {
				shuttingDown = true
			}
			continue
		}

		upd := &collectors.Update{
			Items: make(map[salmon.ItemKey]*salmon.Item, len(sysUpd.Units)),
		}

		// On the first update, also go over the config rules, and for those which are
		// about some specific unit, make sure that it's present in the sysUpd.Units
		// (if it's not actually present from the provider, create a fake one, with
		// the state UnitStateNotSentBySystemd)
		if firstUpdate {
			for _, rule := range c.params.Config.UnitRules {
				// If the rule isn't about a specific unit, we can't make use of it here.
				if rule.Name == "" {
					continue
				}

				// If the unit is present in the actual update from systemd, nothing to do
				if _, ok := sysUpd.Units[rule.Name]; ok {
					continue
				}

				// The unit is not present in the actual update from systemd, so let's
				// add the fake one.

				sysUpd.Units[rule.Name] = &Unit{
					Name:  rule.Name,
					State: UnitStateNotSentBySystemd,
				}
			}

			firstUpdate = false
		}

		for k, unit := range sysUpd.Units {
			// If unit was removed, create a fake one saying that it's not sent
			if unit == nil {
				unit = &Unit{
					Name:  k,
					State: UnitStateNotSentBySystemd,
				}
			}

			item := c.getItemFromUnit(unit)
			if item == nil {
				// We're not interested in this unit.
				continue
			}

			upd.Items[item.Key] = item
		}

		// If the update is empty, don't send it.
		if len(upd.Items) == 0 {
			continue
		}

		if !c.sendUpdate(upd) {
			shuttingDown = true
		}
	}
}

// sendUpdate forwards an update unless shutdown has started. Returning false
// tells run to stop publishing while it drains the provider channel.
func (c *Collector) sendUpdate(update *collectors.Update) bool {
	select {
	case c.params.Common.UpdatesChan <- update:
		return true
	case <-c.teardown:
		return false
	}
}

func (c *Collector) itemKeyFromSystemdName(name string) salmon.ItemKey {
	return salmon.ItemKey(c.params.Common.ID + "." + name)
}

func (c *Collector) getItemFromUnit(unit *Unit) *salmon.Item {
	for _, rule := range c.params.Config.UnitRules {
		if rule.Name != "" && rule.Name != unit.Name {
			continue
		}

		if rule.Type != "" && !strings.HasSuffix(unit.Name, "."+rule.Type) {
			continue
		}

		// This rule applies to the given unit

		item := &salmon.Item{
			Key: c.itemKeyFromSystemdName(unit.Name),
		}

		for i, cond := range rule.Conds {
			if cond.State != "" && cond.State != unit.State {
				continue
			}

			// Found the matching condition, so use its result
			item.State = cond.Result
			item.Comment = fmt.Sprintf("rule: {%s}, unit state: %s, applied condition #%d %+v", unitRuleFilterString(&rule), unit.State, i, cond)
			break
		}

		if item.State == "" {
			// By default, assume error
			item.State = salmon.ItemStateError
			item.Comment = fmt.Sprintf("rule: {%s}, unit state: %s, did not find matching condition", unitRuleFilterString(&rule), unit.State)
		}

		return item
	}

	// Found no rule that applies to the given unit, it means we're not
	// interested in this unit at all.
	return nil
}

func unitRuleFilterString(rule *ConfigUnitRule) string {
	var sb strings.Builder

	if rule.Name != "" {
		if sb.Len() > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("name=")
		sb.WriteString(rule.Name)
	}

	if rule.Type != "" {
		if sb.Len() > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("type=")
		sb.WriteString(rule.Type)
	}

	if sb.Len() == 0 {
		return "default"
	}

	return sb.String()
}
