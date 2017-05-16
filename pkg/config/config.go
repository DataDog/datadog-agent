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
	Datadog.SetDefault("hostname", "")
	Datadog.SetDefault("confd_path", defaultConfdPath)
	Datadog.SetDefault("additional_checksd", defaultAdditionalChecksPath)
	Datadog.SetDefault("use_dogstatsd", true)
	Datadog.SetDefault("dogstatsd_port", 8125)
	Datadog.SetDefault("dogstatsd_buffer_size", 1024)
	Datadog.SetDefault("forwarder_timeout", 20)
	Datadog.SetDefault("dogstatsd_non_local_traffic", false)
	Datadog.SetDefault("dogstatsd_socket_path", "")

	// ENV vars bindings
	Datadog.BindEnv("api_key")
	Datadog.BindEnv("dd_url")
	Datadog.BindEnv("dogstatsd_socket_path")
}
