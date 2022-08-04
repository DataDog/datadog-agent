# Atomic Stats

Agent components commonly need to track some runtime statistics about their operation, such as event counters.
These statistics are often updated atomically, but need to be marshaled into various other forms for display in expvars, `agent status`, and so on.

This package package supports marshalling such structs into `map[string]interface{}`, including support for reading `go.uber.org/atomic` values.

## Usage

To use it, create a struct containing the stats you want to track, and tag the fields with `stats:""`:

```go
type telemetry struct {
	TracesReceived *atomic.Int64 `stats:""`
	TracesFiltered *atomic.Int64 `stats:""`
}
```

The struct can have any number of additional fields without the `stats` tag -- this package will ignore them.

To generate the map of telemetry data when required, call `atomicstats.Report(tlm)`, passing an pointer to an instance of your struct type.
