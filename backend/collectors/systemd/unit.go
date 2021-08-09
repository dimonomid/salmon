package systemd

// UnitState is almost the same as systemd unit's ActiveState, but it also has
// one special value: "not-sent-by-systemd". This value is used only for the
// units which has explicit rule in the config (with the Name set), and it's
// used when systemd did not send any info about this unit. This can happen
// e.g. if the unit was deleted.
type UnitState string

const UnitStateNotSentBySystemd UnitState = "not-sent-by-systemd"

type Unit struct {
	Name  string
	State UnitState
}
