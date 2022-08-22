package telemetry

const (
	// common prefix used by all options
	optPrefix = "_"

	// OptStatsd designates a metric that should be emitted using statsd
	OptStatsd = "_statsd"

	// OptExpvar designates a metric that should be emitted using expvar
	OptExpvar = "_expvar"

	// OptTelemetry designates a metric that should be emitted as agent payload telemetry
	OptTelemetry = "_telemetry"

	// OptGauge represents a gauge-type metric
	OptGauge = "_gauge"

	// OptCounter represents a counter-type metric
	OptCounter = "_counter"

	// OptMonotonic designates a metric of monotonic type.
	// In this case the reporters will only emmit the delta
	OptMonotonic = "_monotonic"
)
