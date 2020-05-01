package config

// Config holds the configuration that allows the span interpreter
// to interpret and enrich various span types.
type Config struct {
	ServiceIdentifiers []string `mapstructure:"service_identifiers"`
}

// DefaultInterpreterConfig creates the default config
func DefaultInterpreterConfig() *Config {
	return &Config{
		ServiceIdentifiers: []string{"db.instance"},
	}
}
