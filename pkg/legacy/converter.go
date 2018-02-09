// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

	if proxy, err := buildProxySettings(agentConfig); err == nil {
		config.Datadog.Set("proxy", proxy)
	}

	if enabled, err := isAffirmative(agentConfig["skip_ssl_validation"]); err == nil {
		config.Datadog.Set("skip_ssl_validation", enabled)
	}

	config.Datadog.Set("api_key", agentConfig["api_key"])
	config.Datadog.Set("hostname", agentConfig["hostname"])

	if enabled, err := isAffirmative(agentConfig["apm_enabled"]); err == nil && !enabled {
		// apm is enabled by default through the check config file `apm.yaml.default`
		config.Datadog.Set("apm_enabled", false)
	}

	if enabled, err := isAffirmative(agentConfig["process_agent_enabled"]); err == nil && !enabled {
		// process agent is enabled in datadog.yaml
		config.Datadog.Set("process_agent_enabled", false)
	}

	config.Datadog.Set("tags", strings.Split(agentConfig["tags"], ","))

	if value, err := strconv.Atoi(agentConfig["forwarder_timeout"]); err == nil {
		config.Datadog.Set("forwarder_timeout", value)
	}

	if value, err := strconv.Atoi(agentConfig["default_integration_http_timeout"]); err == nil {
		config.Datadog.Set("default_integration_http_timeout", value)
	}

	// TODO: collect_ec2_tags

	// config.Datadog has a default value for this, do nothing if the value is empty
	if agentConfig["additional_checksd"] != "" {
		config.Datadog.Set("additional_checksd", agentConfig["additional_checksd"])
	}

	// TODO: exclude_process_args
	config.Datadog.Set("histogram_aggregates", buildHistogramAggregates(agentConfig))

	// TODO: histogram_percentiles

	if agentConfig["service_discovery_backend"] == "docker" {
		// `docker` is the only possible value also on the Agent v5
		dockerListener := config.Listeners{Name: "docker"}
		config.Datadog.Set("listeners", []config.Listeners{dockerListener})
	}

	if providers, err := buildConfigProviders(agentConfig); err == nil {
		config.Datadog.Set("config_providers", providers)
	}

	// config.Datadog has a default value for this, do nothing if the value is empty
	if agentConfig["sd_template_dir"] != "" {
		config.Datadog.Set("autoconf_template_dir", agentConfig["sd_template_dir"])
	}

	if enabled, err := isAffirmative(agentConfig["use_dogstatsd"]); err == nil {
		config.Datadog.Set("use_dogstatsd", enabled)
	}

	if value, err := strconv.Atoi(agentConfig["dogstatsd_port"]); err == nil {
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

	// config.Datadog has a default value for this, do nothing if the value is empty
	if agentConfig["disable_file_logging"] != "" {
		config.Datadog.Set("disable_file_logging", agentConfig["disable_file_logging"])
	}

	if enabled, err := isAffirmative(agentConfig["log_to_syslog"]); err == nil {
		config.Datadog.Set("log_to_syslog", enabled)
	}
	config.Datadog.Set("syslog_uri", buildSyslogURI(agentConfig))

	if enabled, err := isAffirmative(agentConfig["collect_instance_metadata"]); err == nil {
		config.Datadog.Set("enable_metadata_collection", enabled)
	}

	if enabled, err := isAffirmative(agentConfig["enable_gohai"]); err == nil {
		config.Datadog.Set("enable_gohai", enabled)
	}

	//Set the Trace based configs. These should all have default values in the Trace Agent

	if enabled, err := isAffirmative(agentConfig["apm_enabled"]); err == nil {
		config.Datadog.Set("apm_config.enabled", enabled)
	}

	if enabled, err := isAffirmative(agentConfig["non_local_traffic"]); err == nil {
		config.Datadog.Set("apm_config.apm_non_local_traffic", enabled)
	}

	if agentConfig["env"] != "" {
		config.Datadog.Set("apm_config.env", agentConfig["env"])
	}

	if value, err := strconv.Atoi(agentConfig["max_traces_per_second"]); err == nil {
		config.Datadog.Set("apm_config.max_traces_per_second", value)
	}

	if value, err := strconv.Atoi(agentConfig["extra_sample_rate"]); err == nil {
		config.Datadog.Set("apm_config.extra_sample_rate", value)
	}

	if value, err := strconv.Atoi(agentConfig["connection_limit"]); err == nil {
		config.Datadog.Set("apm_config.connection_limit", value)
	}

	if value, err := strconv.Atoi(agentConfig["receiver_port"]); err == nil {
		config.Datadog.Set("apm_config.receiver_port", value)
	}

	if agentConfig["resource"] != "" {
		config.Datadog.Set("apm_config.ignore_resource", agentConfig["resource"])
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

func buildProxySettings(agentConfig Config) (map[string]string, error) {
	proxyHost := agentConfig["proxy_host"]

	proxyMap := make(map[string]string)

	if proxyHost == "" {
		// this is expected, not an error
		return nil, nil
	}

	var err error
	var u *url.URL

	if u, err = url.Parse(proxyHost); err != nil {
		return nil, fmt.Errorf("unable to import value of settings 'proxy_host': %v", err)
	}

	// set scheme if missing
	if u.Scheme == "" {
		u, _ = url.Parse("http://" + proxyHost)
	}

	if agentConfig["proxy_port"] != "" {
		u.Host = u.Host + ":" + agentConfig["proxy_port"]
	}

	if user := agentConfig["proxy_user"]; user != "" {
		if pass := agentConfig["proxy_password"]; pass != "" {
			u.User = url.UserPassword(user, pass)
		} else {
			u.User = url.User(user)
		}
	}

	proxyMap["http"] = u.String()
	proxyMap["https"] = u.String()

	return proxyMap, nil

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

func buildConfigProviders(agentConfig Config) ([]config.ConfigurationProviders, error) {
	// the list of SD_CONFIG_BACKENDS supported in v5
	SdConfigBackends := map[string]struct{}{
		"etcd":   {},
		"consul": {},
		"zk":     {},
	}

	if _, found := SdConfigBackends[agentConfig["sd_config_backend"]]; !found {
		return nil, fmt.Errorf("configuration backend %s is invalid", agentConfig["sd_config_backend"])
	}

	url := agentConfig["sd_backend_host"]
	if agentConfig["sd_backend_port"] != "" {
		url = url + ":" + agentConfig["sd_backend_port"]
	}

	cp := config.ConfigurationProviders{
		Username:    agentConfig["sd_backend_username"],
		Password:    agentConfig["sd_backend_password"],
		TemplateURL: url,
		Polling:     true,
	}

	// v5 supported only one configuration provider at a time
	switch agentConfig["sd_config_backend"] {
	case "etcd":
		cp.Name = "etcd"
	case "consul":
		cp.Name = "consul"
		cp.Token = agentConfig["consul_token"]
	case "zk":
		cp.Name = "zookeeper" // name is different in v6
	}

	return []config.ConfigurationProviders{cp}, nil
}

func buildHistogramAggregates(agentConfig Config) []string {
	configValue := agentConfig["histogram_aggregates"]

	var histogramBuild []string
	// The valid values for histogram_aggregates as defined in agent5
	validValues := []string{"min", "max", "median", "avg", "sum", "count"}

	if configValue == "" {
		return nil
	}
	configValue = strings.Replace(configValue, " ", "", -1)
	result := strings.Split(configValue, ",")

	for _, res := range result {
		found := false
		for _, val := range validValues {
			if res == val {
				histogramBuild = append(histogramBuild, res)
				found = true
				break
			}
		}
		if !found {
			// print the value skipped because invalid value
			fmt.Println("warning: ignored histogram aggregate", res, "is invalid")
		}
	}

	return histogramBuild
}
