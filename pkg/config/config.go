package config

import (
	"time"

	"github.com/spf13/viper"
)

// Datadog is the global configuration object
var Datadog = viper.New()

// MetadataProviders helps unmarshalling `metadata_providers` config param
type MetadataProviders struct {
	Name     string        `mapstructure:"name"`
	Interval time.Duration `mapstructure:"interval"`
}

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
	Datadog.SetDefault("dogstatsd_buffer_size", 1024*8) // 8KB buffer
	Datadog.SetDefault("forwarder_timeout", 20)
	Datadog.SetDefault("dogstatsd_non_local_traffic", false)
	Datadog.SetDefault("dogstatsd_socket", "")
	Datadog.SetDefault("dogstatsd_stats_enable", false)
	Datadog.SetDefault("dogstatsd_stats_buffer", 10)

	// ENV vars bindings
	Datadog.BindEnv("api_key")
	Datadog.BindEnv("dd_url")
	Datadog.BindEnv("dogstatsd_socket")
	Datadog.BindEnv("dogstatsd_non_local_traffic")
}
