package loader

import "github.com/DataDog/datadog-agent/pkg/collector/check"

// ConfigProvider is the interface that wraps the Collect method
//
// Collect is responsible of populating a list of CheckConfig instances
// by retrieving configuration patterns from external resources: files
// on disk, databases, environment variables are just few examples.
//
// Any type implementing the interface will take care of any dependency
// or data needed to access the resource providing the configuration.
type ConfigProvider interface {
	Collect() ([]check.Config, error)
}

// CheckLoader is the interface wrapping the operations to load a check from
// different sources, like Python modules or Go objects.
//
// A check is loaded for every `instance` found in the configuration file.
// Load is supposed to break down instances and return different checks.
type CheckLoader interface {
	Load(config check.Config) ([]check.Check, error)
}
