package config

import "github.com/spf13/viper"

// Datadog is the global configuration object
var Datadog = viper.New()

func init() {
	// config identifiers
	Datadog.SetConfigName("datadog")
	Datadog.SetEnvPrefix("DD")

	// configuration defaults
	Datadog.SetDefault("dd_url", "http://localhost:17123")

	// ENV vars bindings
	Datadog.BindEnv("api_key")
}
