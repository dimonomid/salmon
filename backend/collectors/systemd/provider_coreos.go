package systemd

import (
	"context"
	"fmt"
	"sync"
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

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
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
		params: params,
		conn:   conn,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}

	go p.run()

	return p, nil
}

func (p *ProviderCoreos) Close() {
	p.closeOnce.Do(func() { close(p.stop) })
	<-p.done
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
	p.runSubscription(updatesCh, errCh, p.conn.Close)
}

// runSubscription translates go-systemd subscription events until teardown.
// Keeping this loop separate from DBus connection setup lets its lifecycle be
// tested with controlled channels.
func (p *ProviderCoreos) runSubscription(
	updatesCh <-chan map[string]*dbus.UnitStatus,
	errCh <-chan error,
	closeConn func(),
) {
	defer close(p.done)
	defer close(p.params.Common.UnitUpdatesChan)
	defer closeConn()
	for {
		select {
		case <-p.stop:
			return
		case upd, ok := <-updatesCh:
			if !ok {
				updatesCh = nil
				continue
			}
			m := make(map[string]*Unit, len(upd))
			for k, v := range upd {
				if v == nil {
					// Unit was removed
					m[k] = nil
					continue
				}

				m[k] = &Unit{
					Name:     k,
					State:    UnitState(v.ActiveState),
					SubState: v.SubState,
				}
			}

			if !p.sendUpdate(&UnitUpdate{
				Units: m,
			}) {
				return
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if !p.sendUpdate(&UnitUpdate{
				Err: err,
			}) {
				return
			}
		}
	}
}

// sendUpdate preserves provider backpressure during normal operation while
// allowing Close to interrupt a blocked send.
func (p *ProviderCoreos) sendUpdate(msg *UnitUpdate) bool {
	select {
	case p.params.Common.UnitUpdatesChan <- msg:
		return true
	case <-p.stop:
		return false
	}
}
