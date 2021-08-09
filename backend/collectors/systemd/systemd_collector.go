package systemd

import (
	"fmt"
	"strings"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors"
)

type Collector struct {
	params CollectorParams

	pr       Provider
	torndown chan struct{}
}

var _ collectors.Collector = &Collector{}

type CollectorParams struct {
	Common collectors.Params

	Config Config
}

func NewCollector(params CollectorParams) (*Collector, error) {
	providerUpdCh := make(chan *UnitUpdate, 16)

	pr, err := createProvider(providerUpdCh)
	if err != nil {
		return nil, err
	}

	c := &Collector{
		params:   params,
		pr:       pr,
		torndown: make(chan struct{}),
	}

	go c.run(providerUpdCh)

	return c, nil
}

func (c *Collector) Close() {
	// Close the provider first.
	c.pr.Close()

	// Once the provider teardown is done, it also closes providerUpdCh, which
	// results in the run goroutine exiting the loop and closing the c.torndown
	// channel, so that's what we wait on next.
	<-c.torndown
}

func (c *Collector) run(providerUpdCh chan *UnitUpdate) {
	firstUpdate := true

	for sysUpd := range providerUpdCh {
		if sysUpd.Err != nil {
			// TODO: timeout on send
			c.params.Common.UpdatesChan <- &collectors.Update{
				Err: fmt.Errorf("got error from systemd conn: %w", sysUpd.Err),
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

		// TODO: timeout on send
		c.params.Common.UpdatesChan <- upd
	}

	close(c.torndown)
}

func (c *Collector) itemKeyFromSystemdName(name string) salmon.ItemKey {
	return salmon.ItemKey(c.params.Common.ID + "." + name)
}

func createProvider(updCh chan *UnitUpdate) (Provider, error) {
	pr, err := NewProviderCoreos(ProviderCoreosParams{
		Common: ProviderParams{
			UnitUpdatesChan: updCh,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating coreos systemd provider: %w", err)
	}

	return pr, nil
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
