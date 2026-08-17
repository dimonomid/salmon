package main

import (
	"encoding/json"
	"fmt"

	"github.com/benbjohnson/clock"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/wsclient"
)

// aquascopeCore contains AquaScope's event processing without any systray or
// process-lifecycle concerns, making it usable from end-to-end tests.
type aquascopeCore struct {
	incidentState   *incidentState
	statusWebserver *statusWebserver
	notifications   notificator
	onIconState     func(overallState)
}

type aquascopeCoreParams struct {
	Config        wsclient.Config
	StatePath     string
	Notifications notificator
	OnIconState   func(overallState)
	Clock         clock.Clock
}

func newAquascopeCore(params aquascopeCoreParams) (*aquascopeCore, error) {
	incidentState, err := newIncidentState(params.StatePath)
	if err != nil {
		return nil, err
	}

	core := &aquascopeCore{
		incidentState: incidentState,
		notifications: params.Notifications,
		onIconState:   params.OnIconState,
	}
	core.statusWebserver = newStatusWebserver(statusWebserverParams{
		OnSnooze:   incidentState.Snooze,
		OnUnsnooze: incidentState.Unsnooze,
	})
	incidentState.OnUpdate = core.onIncidentUpdate

	if params.Clock == nil {
		params.Clock = clock.New()
	}
	_, err = wsclient.NewCombiner(wsclient.CombinerParams{
		Config:                  params.Config,
		OngoingIncidentsHandler: core.onNotification,
		Clock:                   params.Clock,
	})
	if err != nil {
		return nil, err
	}
	return core, nil
}

func (c *aquascopeCore) onIncidentUpdate(snapshot incidentSnapshot) {
	c.statusWebserver.SetOngoingIncidents(snapshot)
	if c.onIconState != nil {
		c.onIconState(getOverallStateFromItems(snapshot.Alerting))
	}
}

func (c *aquascopeCore) onNotification(notif *salmon.Notification) {
	snapshot := c.incidentState.Update(notif.OngoingIncidents.Total)

	d, _ := json.MarshalIndent(notif, "", "  ")
	fmt.Println(string(d))

	if c.notifications != nil {
		for _, item := range notif.OngoingIncidents.Added {
			if c.incidentState.IsSnoozed(string(item.Key)) {
				continue
			}
			c.notifications.Push(string(item.State)+": "+string(item.Key), item.Comment)
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
