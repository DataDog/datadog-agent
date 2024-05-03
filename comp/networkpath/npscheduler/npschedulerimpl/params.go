package npschedulerimpl

// TODO: Remove if not needed

type TracerouteRunnerType int

const (
	ClassicTraceroute TracerouteRunnerType = iota
	SimpleTraceroute
)

// Params provides the kind of agent we're instantiating npscheduler for
type Params struct {
	Enabled          bool
	TracerouteRunner TracerouteRunnerType
}
