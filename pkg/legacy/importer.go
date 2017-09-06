// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package legacy

import ini "gopkg.in/ini.v1"

// Config is a simple key/value representation of the legacy agentConfig
// dictionary
type Config map[string]string

// GetAgentConfig reads `datadog.conf` and returns a map that contains the same
// values as the agentConfig dictionary returned by `get_config()` in config.py
func GetAgentConfig(datadogConfPath string) (Config, error) {
	config := make(map[string]string)
	iniFile, err := ini.Load(datadogConfPath)
	if err != nil {
		return config, err
	}

	main, err := iniFile.GetSection("Main")
	if err != nil {
		return config, err
	}

	// Grab the values needed to do a comparison of the Go vs Python algorithm.
	// Note: we'll only import a subset of these values.
	supportedValues := []string{
		"dd_url",
		"proxy_host",
		"proxy_port",
		"proxy_user",
		"proxy_password",
		"skip_ssl_validation",
		"api_key",
		"hostname",
		"apm_enabled",
		"tags",
		"forwarder_timeout",
		"default_integration_http_timeout",
		"collect_ec2_tags",
		"additional_checksd",
		"exclude_process_args",
		"histogram_aggregates",
		"histogram_percentiles",
		"service_discovery_backend",
		"use_dogstatsd",
		"dogstatsd_port",
		"dogstatsd_target",
		"statsd_metric_namespace",
		"log_level",
		"collector_log_file",
		"log_to_syslog",
		"log_to_event_viewer",
		"syslog_host",
		"syslog_port",
		"collect_instance_metadata",
		"listen_port",
		"non_local_traffic",
		"create_dd_check_tags",
		"collect_instance_metadata",
		"proxy_forbid_method_switch",
		"collect_orchestrator_tags",
		"gce_updated_hostname",
		"use_curl_http_client",
		"bind_host",
	}

	for _, supportedValue := range supportedValues {
		if value, err := main.GetKey(supportedValue); err == nil {
			config[supportedValue] = value.String()
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
	config["histogram_aggregates"] = "['max', 'median', 'avg', 'count']"
	config["histogram_percentiles"] = "[0.95]"
	config["endpoints"] = "{}"
	config["version"] = "5.18.0"
	config["proxy_settings"] = "{'host': 'my-proxy.com', 'password': 'password', 'port': 3128, 'user': 'user'}"

	return config, nil
}
