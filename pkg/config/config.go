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

	// Configuration defaults
	// Agent
	Datadog.SetDefault("dd_url", "http://localhost:17123")
	Datadog.SetDefault("proxy", "")
	Datadog.SetDefault("skip_ssl_validation", false)
	Datadog.SetDefault("hostname", "")
	Datadog.SetDefault("conf_path", ".")
	Datadog.SetDefault("confd_path", defaultConfdPath)
	Datadog.SetDefault("additional_checksd", defaultAdditionalChecksPath)
	Datadog.SetDefault("log_file", defaultLogPath)
	Datadog.SetDefault("log_level", "info")
	Datadog.SetDefault("cmd_sock", "/tmp/agent.sock")
	// BUG(massi): make the listener_windows.go module actually use the following:
	Datadog.SetDefault("cmd_pipe_name", `\\.\pipe\ddagent`)
	Datadog.SetDefault("check_runners", int64(4))
	Datadog.SetDefault("forwarder_timeout", 20)
	// Dogstatsd
	Datadog.SetDefault("use_dogstatsd", true)
	Datadog.SetDefault("dogstatsd_port", 8125)
	Datadog.SetDefault("dogstatsd_buffer_size", 1024*8) // 8KB buffer
	Datadog.SetDefault("dogstatsd_non_local_traffic", false)
	Datadog.SetDefault("dogstatsd_socket", "") // Notice: empty means feature disabled
	Datadog.SetDefault("dogstatsd_stats_enable", false)
	Datadog.SetDefault("dogstatsd_stats_buffer", 10)
	// JMX
	Datadog.SetDefault("jmx_pipe_path", defaultJMXPipePath)
	Datadog.SetDefault("jmx_pipe_name", "dd-auto_discovery")
	// Autoconfig
	Datadog.SetDefault("autoconf_template_dir", "/datadog/check_configs")

	// ENV vars bindings
	Datadog.BindEnv("api_key")
	Datadog.BindEnv("dd_url")
	Datadog.BindEnv("cmd_sock")
	Datadog.BindEnv("conf_path")
	Datadog.BindEnv("dogstatsd_socket")
	Datadog.BindEnv("dogstatsd_non_local_traffic")
}
