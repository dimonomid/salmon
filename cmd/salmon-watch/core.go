package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/wsclient"
)

// salmonWatchCore contains Salmon Watch's event processing without any systray or
// process-lifecycle concerns, making it usable from end-to-end tests.
type salmonWatchCore struct {
	incidentState   *incidentState
	statusWebserver *statusWebserver
	notifications   notificator
	onIconState     func(trayState)
	// combiner owns all outbound Salmon connections and is closed with the core.
	combiner          *wsclient.Combiner
	serverStatusesMtx sync.RWMutex
	serverStatuses    map[string]serverStatus
	// serverIDs preserves the order of servers in the Salmon Watch configuration.
	serverIDs []string
}

type serverStatus struct {
	// ID identifies the configured Salmon server.
	ID string `json:"id"`
	// Connected reports the latest connection state.
	Connected bool `json:"connected"`
	// ConnectionChangedAt is when the connection most recently changed state.
	ConnectionChangedAt *time.Time `json:"connectionChangedAt,omitempty"`
	// LastHeartbeatTime is when the most recent heartbeat was received.
	LastHeartbeatTime *time.Time `json:"lastHeartbeatTime,omitempty"`
	initialized       bool       `json:"-"`
}

// trayState is the already-aggregated incident information needed by the tray
// UI. Snoozed is nil when no snoozed incidents need an icon overlay.
type trayState struct {
	Alerting      overallState
	AlertingCount int
	Snoozed       *overallState
	SnoozedCount  int
}

type salmonWatchCoreParams struct {
	// Config describes the Salmon servers Salmon Watch should connect to.
	Config wsclient.Config
	// StatePath is the location of the persisted snooze state file.
	StatePath string
	// Notifications receives desktop notification events.
	Notifications notificator
	// OnIconState applies the aggregated incident state to the tray UI.
	OnIconState func(trayState)
	// Clock supplies time to all incident and snooze state; it is required.
	Clock clock.Clock
	// ReconnectDelay overrides the production 5*time.Second reconnect delay
	// when non-zero.
	ReconnectDelay time.Duration
	// SnoozeCheckInterval overrides the production 10*time.Second snooze
	// expiration polling interval when non-zero.
	SnoozeCheckInterval time.Duration
}

func newSalmonWatchCore(params salmonWatchCoreParams) (*salmonWatchCore, error) {
	if params.Clock == nil {
		panic("Clock is required")
	}
	if err := params.Config.Validate(); err != nil {
		return nil, err
	}

	var incidentState *incidentState
	var err error
	if params.SnoozeCheckInterval > 0 {
		incidentState, err = newIncidentStateWithInterval(params.StatePath, params.SnoozeCheckInterval, params.Clock)
	} else {
		incidentState, err = newIncidentState(params.StatePath, params.Clock)
	}
	if err != nil {
		return nil, err
	}

	core := &salmonWatchCore{
		incidentState:  incidentState,
		notifications:  params.Notifications,
		onIconState:    params.OnIconState,
		serverStatuses: make(map[string]serverStatus, len(params.Config.Servers)),
		serverIDs:      make([]string, 0, len(params.Config.Servers)),
	}
	for _, server := range params.Config.Servers {
		core.serverStatuses[server.ID] = serverStatus{ID: server.ID}
		core.serverIDs = append(core.serverIDs, server.ID)
	}
	core.statusWebserver = newStatusWebserver(statusWebserverParams{
		OnSnooze:   incidentState.Snooze,
		OnUnsnooze: incidentState.Unsnooze,
	})
	core.publishServerStatuses()
	incidentState.OnUpdate = core.onIncidentUpdate

	core.combiner, err = wsclient.NewCombiner(wsclient.CombinerParams{
		Config:                  params.Config,
		OngoingIncidentsHandler: core.onNotification,
		Clock:                   params.Clock,
		ReconnectDelay:          params.ReconnectDelay,
		ConnectionStatusHandler: core.onConnectionEvent,
	})
	if err != nil {
		incidentState.Close()
		return nil, err
	}
	return core, nil
}

func (c *salmonWatchCore) onConnectionEvent(id string, event wsclient.ConnectionEvent) {
	c.serverStatusesMtx.Lock()
	status := c.serverStatuses[id]
	if event.EventKind == wsclient.EventKindHeartbeat {
		now := event.Time
		status.LastHeartbeatTime = &now
	} else if event.EventKind == wsclient.EventKindConnected || event.EventKind == wsclient.EventKindDisconnected {
		now := event.Time
		connected := event.EventKind == wsclient.EventKindConnected
		if status.initialized && connected == status.Connected {
			c.serverStatusesMtx.Unlock()
			return
		}
		status.Connected = connected
		status.initialized = true
		status.ConnectionChangedAt = &now
		if connected {
			status.LastHeartbeatTime = nil
		}
	}
	c.serverStatuses[id] = status
	c.serverStatusesMtx.Unlock()
	c.publishServerStatuses()
}

func (c *salmonWatchCore) publishServerStatuses() {
	c.serverStatusesMtx.RLock()
	statuses := make([]serverStatus, 0, len(c.serverIDs))
	// Iterate by configured ID rather than over serverStatuses: Go map iteration
	// order is deliberately unstable, while the status UI shows this order.
	for _, id := range c.serverIDs {
		statuses = append(statuses, c.serverStatuses[id])
	}
	c.serverStatusesMtx.RUnlock()
	c.statusWebserver.SetServerStatuses(statuses)
}

// Close stops Salmon Watch's workers and waits for them to exit.
func (c *salmonWatchCore) Close() {
	if c.combiner != nil {
		c.combiner.Close()
	}
	if c.incidentState != nil {
		c.incidentState.Close()
	}
}

func (c *salmonWatchCore) onIncidentUpdate(snapshot incidentSnapshot) {
	c.statusWebserver.SetOngoingIncidents(snapshot)
	if c.onIconState != nil {
		state := trayState{
			Alerting:      getOverallStateFromItems(snapshot.Alerting),
			AlertingCount: len(snapshot.Alerting),
			SnoozedCount:  len(snapshot.Snoozed),
		}
		if len(snapshot.Snoozed) > 0 {
			snoozed := getOverallStateFromItems(snapshot.SnoozedItems())
			state.Snoozed = &snoozed
		}
		c.onIconState(state)
	}
}

func (c *salmonWatchCore) onNotification(notif *salmon.Notification) {
	snapshot := c.incidentState.Update(notif.OngoingIncidents.Total)

	d, _ := json.MarshalIndent(notif, "", "  ")
	fmt.Println(string(d))

	if c.notifications != nil {
		for _, item := range notif.OngoingIncidents.Added {
			if c.incidentState.IsSnoozed(string(item.Key)) {
				continue
			}
			c.notifications.Push(string(item.State)+": "+string(item.Key), item.Details)
		}
		// Do not show desktop notifications for updates to existing incidents:
		// volatile details (such as connection ports in an error) can change
		// repeatedly while the underlying incident is still the same.
		for _, item := range notif.OngoingIncidents.Removed {
			if c.incidentState.IsSnoozed(string(item.Key)) {
				continue
			}
			c.notifications.Push("OK: "+string(item.Key), "")
		}
	}

	state := getOverallStateFromItems(snapshot.Alerting)
	fmt.Println("Overall state:", state)
}
