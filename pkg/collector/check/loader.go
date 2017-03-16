package check

// Loader is the interface wrapping the operations to load a check from
// different sources, like Python modules or Go objects.
//
// A check is loaded for every `instance` found in the configuration file.
// Load is supposed to break down instances and return different checks.
type Loader interface {
	Load(config Config) ([]Check, error)
}
