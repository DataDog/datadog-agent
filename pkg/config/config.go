package config

import (
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/go-ini/ini"
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
	Datadog.SetDefault("dogstatsd_6_enable", false)
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

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints() (map[string][]string, error) {
	return getMultipleEndpoints(Datadog)
}

// getMultipleEndpoints implements the logic to extract the api keys per domain from an agent config
func getMultipleEndpoints(config *viper.Viper) (map[string][]string, error) {
	keysPerDomain := map[string][]string{
		config.GetString("dd_url"): {
			config.GetString("api_key"),
		},
	}

	var additionalEndpoints map[string][]string
	err := config.UnmarshalKey("additional_endpoints", &additionalEndpoints)
	if err != nil {
		return keysPerDomain, err
	}

	// merge additional endpoints into keysPerDomain
	for domain, apiKeys := range additionalEndpoints {
		if _, ok := keysPerDomain[domain]; ok {
			for _, apiKey := range apiKeys {
				keysPerDomain[domain] = append(keysPerDomain[domain], apiKey)
			}
		} else {
			keysPerDomain[domain] = apiKeys
		}
	}

	// dedupe api keys and remove domains with no api keys (or empty ones)
	for domain, apiKeys := range keysPerDomain {
		dedupedAPIKeys := make([]string, 0, len(apiKeys))
		seen := make(map[string]bool)
		for _, apiKey := range apiKeys {
			trimmedAPIKey := strings.TrimSpace(apiKey)
			if _, ok := seen[trimmedAPIKey]; !ok && trimmedAPIKey != "" {
				seen[trimmedAPIKey] = true
				dedupedAPIKeys = append(dedupedAPIKeys, trimmedAPIKey)
			}
		}

		if len(dedupedAPIKeys) > 0 {
			keysPerDomain[domain] = dedupedAPIKeys
		} else {
			log.Infof("No API key provided for domain \"%s\", removing domain from endpoints", domain)
			delete(keysPerDomain, domain)
		}
	}

	return keysPerDomain, nil
}

// ReadLegacyConfig will read the legacy config (needed by dogstatsd6)
func ReadLegacyConfig() error {

	cfg, err := ini.Load(Datadog.GetString("conf_path"))
	if err != nil {
		return err
	}
	main, err := cfg.GetSection("Main")
	if err != nil {
		return err
	}

	if v := main.Key("hostname").MustString(""); v != "" {
		//set value
		Datadog.Set("hostname", v)
	}
	if v := main.Key("api_key").MustString(""); v != "" {
		//set value
		Datadog.Set("api_key", v)
	}
	if v := main.Key("dd_url").MustString("http://localhost:17123"); v != "" {
		//set value
		Datadog.Set("dd_url", v)
	}
	if v := main.Key("use_dogstatsd").MustBool(true); v {
		//set value
		Datadog.Set("use_dogstatsd", v)
	}
	if v := main.Key("dogstatsd6_enable").MustBool(false); v {
		//set value
		Datadog.Set("dogstatsd6_enable", v)
	}
	if v := main.Key("dogstatsd_port").MustInt64(8125); v != 8125 {
		//set value
		Datadog.Set("dogstatsd_port", v)
	}
	if v := main.Key("dogstatsd_buffer_size").MustInt64(1024 * 8); v != (1024 * 8) {
		//set value
		Datadog.Set("dogstatsd_buffer_size", v)
	}
	if v := main.Key("dogstatsd_non_local_traffic").MustBool(false); v {
		//set value
		Datadog.Set("dogstatsd_non_local_traffic", v)
	}
	if v := main.Key("dogstatsd_socket").MustString(""); v != "" {
		//set value
		Datadog.Set("dogstatsd_socket", v)
	}
	if v := main.Key("dogstatsd_stats_enable").MustBool(false); v {
		//set value
		Datadog.Set("dogstatsd_stats_enable", v)
	}
	if v := main.Key("dogstatsd_stats_buffer").MustInt64(10); v != 0 {
		//set value
		Datadog.Set("dogstatsd_stats_buffer", v)
	}

	return err
}
