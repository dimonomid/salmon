package systemd

type Provider interface {
	// Close tears down the provider. It's synchronous: by the time Close returns,
	// all resources held by the Provider are already reclaimed.
	Close()
}

type ProviderParams struct {
	// UnitUpdatesChan is the channel where Provider should send unit updates.
	// The first update must contain info about _all_ existing units; otherwise
	// it's possible to get some initial transient incident reports.
	UnitUpdatesChan chan<- *UnitUpdate
}

type UnitUpdate struct {
	Units map[string]*Unit
	Err   error
}
