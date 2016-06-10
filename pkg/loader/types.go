package loader

// CheckConfig is a generic container for configuration files
type CheckConfig struct {
	Name string                 // the name of the check
	Data map[string]interface{} // raw configuration content, unmarshalled from Yaml
}

// ConfigProvider is the interface that wraps the Collect method
//
// Collect is responsible of populating a list of CheckConfig instances
// by retrieving configuration patterns from external resources: files
// on disk, databases, environment variables are just few examples.
//
// Any type implementing the interface will take care of any dependency
// or data needed to access the resource providing the configuration.
type ConfigProvider interface {
	Collect() ([]CheckConfig, error)
}
