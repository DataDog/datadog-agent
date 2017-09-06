// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package legacy

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// FromAgentConfig reads the old agentConfig configuration, converts and merges
// the values into the current config.Datadog object
func FromAgentConfig(agentConfig Config) error {
	config.Datadog.Set("dd_url", agentConfig["dd_url"])

	proxy, err := buildProxySettings(agentConfig)
	if err == nil {
		config.Datadog.Set("proxy", proxy)
	}

	enabled, err := isAffirmative(agentConfig["skip_ssl_validation"])
	if err == nil {
		config.Datadog.Set("skip_ssl_validation", enabled)
	}

	config.Datadog.Set("api_key", agentConfig["api_key"])
	config.Datadog.Set("hostname", agentConfig["hostname"])

	enabled, err = isAffirmative(agentConfig["apm_enabled"])
	if err == nil && !enabled {
		// apm is enabled by default through the check config file
		// TODO: disable APM check
	}

	config.Datadog.Set("tags", strings.Split(agentConfig["tags"], ","))

	value, err := strconv.Atoi(agentConfig["forwarder_timeout"])
	if err == nil {
		config.Datadog.Set("forwarder_timeout", value)
	}

	// TODO: default_integration_http_timeout
	// TODO: collect_ec2_tags

	// config.Datadog has a default value for this, do nothing if the value is empty
	if agentConfig["additional_checksd"] != "" {
		config.Datadog.Set("additional_checksd", agentConfig["additional_checksd"])
	}

	// TODO: exclude_process_args
	// TODO: histogram_aggregates
	// TODO: histogram_percentiles

	if agentConfig["service_discovery_backend"] == "docker" {
		// this means SD is enabled on the old config
		// TODO: enable Autodiscovery
	}

	enabled, err = isAffirmative(agentConfig["use_dogstatsd"])
	if err == nil {
		config.Datadog.Set("use_dogstatsd", enabled)
	}

	value, err = strconv.Atoi(agentConfig["dogstatsd_port"])
	if err == nil {
		config.Datadog.Set("dogstatsd_port", value)
	}

	// TODO: statsd_metric_namespace

	// config.Datadog has a default value for this, do nothing if the value is empty
	if agentConfig["log_level"] != "" {
		config.Datadog.Set("log_level", agentConfig["log_level"])
	}

	// config.Datadog has a default value for this, do nothing if the value is empty
	if agentConfig["collector_log_file"] != "" {
		config.Datadog.Set("log_file", agentConfig["collector_log_file"])
	}

	enabled, err = isAffirmative(agentConfig["log_to_syslog"])
	if err == nil {
		config.Datadog.Set("log_to_syslog", enabled)
	}
	config.Datadog.Set("syslog_uri", buildSyslogURI(agentConfig))

	enabled, err = isAffirmative(agentConfig["collect_instance_metadata"])
	if err == nil {
		config.Datadog.Set("enable_metadata_collection", enabled)
	}

	return nil
}

func isAffirmative(value string) (bool, error) {
	if value == "" {
		return false, fmt.Errorf("value is empty")
	}

	v := strings.ToLower(value)
	return v == "true" || v == "yes" || v == "1", nil
}

func buildProxySettings(agentConfig Config) (string, error) {
	if agentConfig["proxy_host"] == "" {
		// this is expected, not an error
		return "", nil
	}

	u, err := url.Parse(agentConfig["proxy_host"])
	if err != nil {
		return "", fmt.Errorf("unable to import value of settings 'proxy_host': %v", err)
	}

	if agentConfig["proxy_port"] != "" {
		u.Host = u.Host + ":" + agentConfig["proxy_port"]
	}

	user := agentConfig["proxy_user"]
	pass := agentConfig["proxy_password"]
	if user != "" {
		if pass != "" {
			u.User = url.UserPassword(user, pass)
		} else {
			u.User = url.User(user)
		}
	}

	return u.String(), nil
}

func buildSyslogURI(agentConfig Config) string {
	host := agentConfig["syslog_host"]

	if host == "" {
		// this is expected, not an error
		return ""
	}

	if agentConfig["syslog_port"] != "" {
		host = host + ":" + agentConfig["syslog_port"]
	}

	return host
}
