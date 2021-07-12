package telemetry

// Options for telemetry metrics.
// Creating an Options struct without specifying any of its fields should be the
// equivalent of using the DefaultOptions var.
type Options struct {
	// NoDoubleUnderscoreSep is set to true when you don't want to
	// separate the subsystem and the name with a double underscore separator.
	NoDoubleUnderscoreSep bool
}

// DefaultOptions for telemetry metrics which don't need to specify any option.
var DefaultOptions = Options{
	// By default, we want to separate the subsystem and the metric name with a
	// double underscore to be able to replace it later in the process.
	NoDoubleUnderscoreSep: false,
}
