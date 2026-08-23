package systemd

type Provider interface {
	// Close tears down the provider. It is synchronous: before returning, the
	// provider must stop all sends, close the UnitUpdatesChan supplied in
	// ProviderParams, and reclaim all resources it holds. Collector shutdown
	// waits for that channel to close.
	Close()
}

type ProviderParams struct {
	// UnitUpdatesChan is the channel where Provider should send unit updates.
	// The provider owns the sending side and must close the channel during Close
	// after it has stopped all sends. The first update must contain info about
	// _all_ existing units; otherwise it's possible to get some initial transient
	// incident reports.
	UnitUpdatesChan chan<- *UnitUpdate
}

type UnitUpdate struct {
	Units map[string]*Unit
	Err   error
}
