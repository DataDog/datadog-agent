// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package legacy

// `aws-sdk-go` imports go-ini like this instead of `gopkg.in/ini.v1`, let's do
// the same to avoid checking in the dependency twice with different names.
import (
	"github.com/go-ini/ini"
)

// Config is a simple key/value representation of the legacy agentConfig
// dictionary
type Config map[string]string

var (
	// Note: we'll only import a subset of these values.
	supportedValues = []string{
		"dd_url",
		"proxy_host",
		"proxy_port",
		"proxy_user",
		"proxy_password",
		"skip_ssl_validation",
		"api_key",
		"hostname",
		"tags",
		"forwarder_timeout",
		"default_integration_http_timeout",
		"collect_ec2_tags",
		"additional_checksd",
		"exclude_process_args",
		"histogram_aggregates",
		"histogram_percentiles",
		"service_discovery_backend",
		"sd_config_backend",
		"sd_backend_host",
		"sd_backend_port",
		"sd_backend_username",
		"sd_backend_password",
		"sd_template_dir",
		"consul_token",
		"use_dogstatsd",
		"dogstatsd_port",
		"statsd_metric_namespace",
		"log_level",
		"collector_log_file",
		"log_to_syslog",
		"log_to_event_viewer", // maybe deprecated, ignore for now
		"syslog_host",
		"syslog_port",
		"collect_instance_metadata",
		"listen_port",          // not for 6.0, ignore for now
		"create_dd_check_tags", // not for 6.0, ignore for now
		"non_local_traffic",    // converted for the trace-agent and dogstatsd
		"bind_host",
		"proxy_forbid_method_switch", // deprecated
		"collect_orchestrator_tags",  // deprecated
		"use_curl_http_client",       // deprecated
		"dogstatsd_target",           // deprecated
		"gce_updated_hostname",       // deprecated
		"process_agent_enabled",
		"disable_file_logging",
		"enable_gohai",
		"apm_enabled",
	}
)

// GetAgentConfig reads `datadog.conf` and returns a map that contains the same
// values as the agentConfig dictionary returned by `get_config()` in config.py
func GetAgentConfig(datadogConfPath string) (Config, error) {
	config := make(Config)
	iniFile, err := ini.Load(datadogConfPath)
	if err != nil {
		return config, err
	}

	// get the Main section
	main, err := iniFile.GetSection("Main")
	if err != nil {
		return config, err
	}

	// Grab the values needed to do a comparison of the Go vs Python algorithm.
	for _, supportedValue := range supportedValues {
		if value, err := main.GetKey(supportedValue); err == nil {
			config[supportedValue] = value.String()
		} else {
			// provide an empty default value so we don't need to check for
			// key existence when browsing the old configuration
			config[supportedValue] = ""
		}
	}

	// these are hardcoded in config.py
	config["graphite_listen_port"] = "None"
	config["watchdog"] = "True"
	config["use_forwarder"] = "False" // this doesn't come from the config file
	config["check_freq"] = "15"
	config["utf8_decoding"] = "False"
	config["ssl_certificate"] = "datadog-cert.pem"
	config["use_web_info_page"] = "True"

	// these values are postprocessed in config.py, manually overwrite them
	config["endpoints"] = "{}"
	config["version"] = "5.18.0"
	config["proxy_settings"] = "{'host': 'my-proxy.com', 'password': 'password', 'port': 3128, 'user': 'user'}"
	config["service_discovery"] = "True"

	// add trace agent sections as <section>.<key>
	for _, section := range []string{
		"trace.api",
		"trace.config",
		"trace.ignore",
		"trace.analyzed_rate_by_service",
		"trace.analyzed_spans",
		"trace.concentrator",
		"trace.sampler",
		"trace.receiver",
		"trace.watchdog",
		"trace.writer.services",
		"trace.writer.stats",
		"trace.writer.traces",
	} {
		s, err := iniFile.GetSection(section)
		if err != nil {
			continue
		}
		for k, v := range s.KeysHash() {
			config[section+"."+k] = v
		}
	}

	return config, nil
}
