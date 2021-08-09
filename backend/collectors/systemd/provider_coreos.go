package systemd

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

// Provider implements provider.Provider using github.com/coreos/go-systemd.
// It's not exactly great because it's polling for changes instead of actually
// subscribing to them, so unless it's fixed at some point, perhaps we'll need
// to move to some other library (or implement it).
type ProviderCoreos struct {
	params ProviderCoreosParams

	conn *dbus.Conn

	teardownCh chan chan struct{}
}

var _ Provider = &ProviderCoreos{}

type ProviderCoreosParams struct {
	Common ProviderParams
}

func NewProviderCoreos(params ProviderCoreosParams) (*ProviderCoreos, error) {
	conn, err := dbus.NewWithContext(context.Background())
	if err != nil {
		return nil, fmt.Errorf("creating dbus conn: %w", err)
	}

	p := &ProviderCoreos{
		params:     params,
		conn:       conn,
		teardownCh: make(chan chan struct{}),
	}

	go p.run()

	return p, nil
}

func (p *ProviderCoreos) Close() {
	c := make(chan struct{})
	p.teardownCh <- c
	<-c
}

func (p *ProviderCoreos) run() {
	// NOTE: it's not really great because it is polling ListUnits every second.
	// Actually this go-systemd library has another way of subscribing:
	// dbus.SubscriptionSet, but as of today its Subscribe method uses the same
	// SubscribeUnits with 1s interval under the hood, with a TODO:
	// "Make fully evented by using systemd 209 with properties changed values".
	//
	// So until that's changed, we just use SubscribeUnits directly here; this way
	// we also don't have to specify which units we want to filter (because we actually
	// want all units)
	updatesCh, errCh := p.conn.SubscribeUnits(1 * time.Second)

	for {
		select {
		case upd := <-updatesCh:
			m := make(map[string]*Unit, len(upd))
			for k, v := range upd {
				if v == nil {
					// Unit was removed
					m[k] = nil
					continue
				}

				m[k] = &Unit{
					Name:  k,
					State: UnitState(v.ActiveState),
				}
			}

			p.sendWithTimeout(&UnitUpdate{
				Units: m,
			})
		case err := <-errCh:
			p.sendWithTimeout(&UnitUpdate{
				Err: err,
			})

		case c := <-p.teardownCh:
			p.conn.Close()
			close(p.params.Common.UnitUpdatesChan)
			close(c)
		}
	}
}

func (p *ProviderCoreos) sendWithTimeout(msg *UnitUpdate) {
	select {
	case p.params.Common.UnitUpdatesChan <- msg:
	case <-time.After(1 * time.Second):
		panic("wasn't able to send a message after 1s")
	}
}
