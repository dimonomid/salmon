package core

import (
	"fmt"

	"github.com/dimonomid/salmon/backend/collectors"
	"github.com/dimonomid/salmon/backend/collectors/exec"
	"github.com/dimonomid/salmon/backend/collectors/systemd"
)

func createCollector(
	cfg Collector, commonParams collectors.Params,
) (collectors.Collector, error) {
	if err := CheckID(cfg.ID); err != nil {
		return nil, err
	}

	var ret collectors.Collector

	if cfg.Systemd != nil {
		if ret != nil {
			return nil, fmt.Errorf("config contains more than a single collector")
		}

		c, err := systemd.NewCollector(systemd.CollectorParams{
			Common: commonParams,
			Config: *cfg.Systemd,
		})
		if err != nil {
			return nil, fmt.Errorf("creating systemd collector: %w", err)
		}

		return c, nil
	}

	if cfg.Exec != nil {
		if ret != nil {
			return nil, fmt.Errorf("config contains more than a single collector")
		}

		c, err := exec.NewCollector(exec.CollectorParams{
			Common: commonParams,
			Config: *cfg.Exec,
		})
		if err != nil {
			return nil, fmt.Errorf("creating exec collector: %w", err)
		}

		return c, nil
	}

	return nil, fmt.Errorf("no valid collector configuration")
}

func createCollectors(
	cfgs []Collector, commonParams collectors.Params,
) ([]collectors.Collector, error) {
	ret := make([]collectors.Collector, 0, len(cfgs))

	usedIDs := make(map[string]struct{}, len(cfgs))

	for i, cfg := range cfgs {
		if _, used := usedIDs[cfg.ID]; used {
			return nil, fmt.Errorf("collector config #%d: duplicate id %q", i, cfg.ID)
		}

		usedIDs[cfg.ID] = struct{}{}

		curCommonParams := commonParams
		curCommonParams.ID = cfg.ID
		c, err := createCollector(cfg, curCommonParams)
		if err != nil {
			return nil, fmt.Errorf("creating collector from config #%d: %w", i, err)
		}

		ret = append(ret, c)
	}

	return ret, nil
}
