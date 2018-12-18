// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package legacy

import (
	"encoding/csv"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// FromAgentConfig reads the old agentConfig configuration, converts and merges
// the values into the current configuration object
func FromAgentConfig(agentConfig Config) error {
	configConverter := config.NewConfigConverter()

	if err := extractURLAPIKeys(agentConfig, configConverter); err != nil {
		return err
	}

	if proxy, err := buildProxySettings(agentConfig); err == nil {
		if u, ok := proxy["http"]; ok {
			configConverter.Set("proxy.http", u)
		}
		if u, ok := proxy["https"]; ok {
			configConverter.Set("proxy.https", u)
		}
	}

	if enabled, err := isAffirmative(agentConfig["skip_ssl_validation"]); err == nil {
		configConverter.Set("skip_ssl_validation", enabled)
	}

	configConverter.Set("hostname", agentConfig["hostname"])

	if enabled, err := isAffirmative(agentConfig["process_agent_enabled"]); enabled {
		// process agent is explicitly enabled
		configConverter.Set("process_config.enabled", "true")
	} else if err == nil && !enabled {
		// process agent is explicitly disabled
		configConverter.Set("process_config.enabled", "disabled")
	}

	configConverter.Set("tags", strings.Split(agentConfig["tags"], ","))

	if value, err := strconv.Atoi(agentConfig["forwarder_timeout"]); err == nil {
		configConverter.Set("forwarder_timeout", value)
	}

	if value, err := strconv.Atoi(agentConfig["default_integration_http_timeout"]); err == nil {
		configConverter.Set("default_integration_http_timeout", value)
	}

	if enabled, err := isAffirmative(agentConfig["collect_ec2_tags"]); err == nil {
		configConverter.Set("collect_ec2_tags", enabled)
	}

	// configConverter has a default value for this, do nothing if the value is empty
	if agentConfig["additional_checksd"] != "" {
		configConverter.Set("additional_checksd", agentConfig["additional_checksd"])
	}

	// TODO: exclude_process_args

	histogramAggregates := buildHistogramAggregates(agentConfig)
	if histogramAggregates != nil && len(histogramAggregates) != 0 {
		configConverter.Set("histogram_aggregates", histogramAggregates)
	}

	histogramPercentiles := buildHistogramPercentiles(agentConfig)
	if histogramPercentiles != nil && len(histogramPercentiles) != 0 {
		configConverter.Set("histogram_percentiles", histogramPercentiles)
	}

	if agentConfig["service_discovery_backend"] == "docker" {
		// `docker` is the only possible value also on the Agent v5
		dockerListener := config.Listeners{Name: "docker"}
		configConverter.Set("listeners", []config.Listeners{dockerListener})
	}

	if providers, err := buildConfigProviders(agentConfig); err == nil {
		configConverter.Set("config_providers", providers)
	}

	// configConverter has a default value for this, do nothing if the value is empty
	if agentConfig["sd_template_dir"] != "" {
		configConverter.Set("autoconf_template_dir", agentConfig["sd_template_dir"])
	}

	if enabled, err := isAffirmative(agentConfig["use_dogstatsd"]); err == nil {
		configConverter.Set("use_dogstatsd", enabled)
	}

	if value, err := strconv.Atoi(agentConfig["dogstatsd_port"]); err == nil {
		configConverter.Set("dogstatsd_port", value)
	}

	configConverter.Set("statsd_metric_namespace", agentConfig["statsd_metric_namespace"])

	// configConverter has a default value for this, do nothing if the value is empty
	if agentConfig["log_level"] != "" {
		configConverter.Set("log_level", agentConfig["log_level"])
	}

	// configConverter has a default value for this, do nothing if the value is empty
	if agentConfig["collector_log_file"] != "" {
		configConverter.Set("log_file", agentConfig["collector_log_file"])
	}

	// configConverter has a default value for this, do nothing if the value is empty
	if agentConfig["disable_file_logging"] != "" {
		configConverter.Set("disable_file_logging", agentConfig["disable_file_logging"])
	}

	if enabled, err := isAffirmative(agentConfig["log_to_syslog"]); err == nil {
		configConverter.Set("log_to_syslog", enabled)
	}
	configConverter.Set("syslog_uri", buildSyslogURI(agentConfig))

	if enabled, err := isAffirmative(agentConfig["collect_instance_metadata"]); err == nil {
		configConverter.Set("enable_metadata_collection", enabled)
	}

	if enabled, err := isAffirmative(agentConfig["enable_gohai"]); err == nil {
		configConverter.Set("enable_gohai", enabled)
	}

	if agentConfig["bind_host"] != "" {
		configConverter.Set("bind_host", agentConfig["bind_host"])
	}

	//Trace APM based configurations

	if agentConfig["apm_enabled"] != "" {
		if enabled, err := isAffirmative(agentConfig["apm_enabled"]); err == nil {
			configConverter.Set("apm_config.enabled", enabled)
		}
	}

	if agentConfig["non_local_traffic"] != "" {
		if enabled, err := isAffirmative(agentConfig["non_local_traffic"]); err == nil {
			// trace-agent listen locally by default, convert the config only if configured to listen to more
			configConverter.Set("apm_config.apm_non_local_traffic", enabled)
		}
	}

	configConverter.Set("hostname_fqdn", true)

	return extractTraceAgentConfig(agentConfig, configConverter)
}

func extractTraceAgentConfig(agentConfig Config, configConverter *config.LegacyConfigConverter) error {
	for iniKey, yamlKey := range map[string]string{
		"trace.api.api_key":                                      "apm_config.api_key",
		"trace.api.endpoint":                                     "apm_config.apm_dd_url",
		"trace.config.env":                                       "apm_config.env",
		"trace.config.log_level":                                 "apm_config.log_level",
		"trace.config.log_file":                                  "apm_config.log_file",
		"trace.config.log_throttling":                            "apm_config.log_throttling",
		"trace.concentrator.bucket_size_seconds":                 "apm_config.bucket_size_seconds",
		"trace.concentrator.extra_aggregators":                   "apm_config.extra_aggregators",
		"trace.receiver.receiver_port":                           "apm_config.receiver_port",
		"trace.receiver.connection_limit":                        "apm_config.connection_limit",
		"trace.receiver.timeout":                                 "apm_config.receiver_timeout",
		"trace.sampler.extra_sample_rate":                        "apm_config.extra_sample_rate",
		"trace.sampler.max_traces_per_second":                    "apm_config.max_traces_per_second",
		"trace.sampler.max_events_per_second":                    "apm_config.max_events_per_second",
		"trace.watchdog.max_memory":                              "apm_config.max_memory",
		"trace.watchdog.max_cpu_percent":                         "apm_config.max_cpu_percent",
		"trace.watchdog.max_connections":                         "apm_config.max_connections",
		"trace.watchdog.check_delay_seconds":                     "apm_config.watchdog_check_delay",
		"trace.writer.services.flush_period_seconds":             "apm_config.service_writer.flush_period_seconds",
		"trace.writer.services.update_info_period_seconds":       "apm_config.service_writer.update_info_period_seconds",
		"trace.writer.services.queue_max_age_seconds":            "apm_config.service_writer.queue.max_age_seconds",
		"trace.writer.services.queue_max_bytes":                  "apm_config.service_writer.queue.max_bytes",
		"trace.writer.services.queue_max_payloads":               "apm_config.service_writer.queue.max_payloads",
		"trace.writer.services.exp_backoff_max_duration_seconds": "apm_config.service_writer.queue.exp_backoff_max_duration_seconds",
		"trace.writer.services.exp_backoff_base_milliseconds":    "apm_config.service_writer.queue.exp_backoff_base_milliseconds",
		"trace.writer.services.exp_backoff_growth_base":          "apm_config.service_writer.queue.exp_backoff_growth_base",
		"trace.writer.stats.max_entries_per_payload":             "apm_config.stats_writer.max_entries_per_payload",
		"trace.writer.stats.update_info_period_seconds":          "apm_config.stats_writer.update_info_period_seconds",
		"trace.writer.stats.queue_max_age_seconds":               "apm_config.stats_writer.queue.max_age_seconds",
		"trace.writer.stats.queue_max_bytes":                     "apm_config.stats_writer.queue.max_bytes",
		"trace.writer.stats.queue_max_payloads":                  "apm_config.stats_writer.queue.max_payloads",
		"trace.writer.stats.exp_backoff_max_duration_seconds":    "apm_config.stats_writer.queue.exp_backoff_max_duration_seconds",
		"trace.writer.stats.exp_backoff_base_milliseconds":       "apm_config.stats_writer.queue.exp_backoff_base_milliseconds",
		"trace.writer.stats.exp_backoff_growth_base":             "apm_config.stats_writer.queue.exp_backoff_growth_base",
		"trace.writer.traces.max_spans_per_payload":              "apm_config.trace_writer.max_spans_per_payload",
		"trace.writer.traces.flush_period_seconds":               "apm_config.trace_writer.flush_period_seconds",
		"trace.writer.traces.update_info_period_seconds":         "apm_config.trace_writer.update_info_period_seconds",
		"trace.writer.traces.queue_max_age_seconds":              "apm_config.trace_writer.queue.max_age_seconds",
		"trace.writer.traces.queue_max_bytes":                    "apm_config.trace_writer.queue.max_bytes",
		"trace.writer.traces.queue_max_payloads":                 "apm_config.trace_writer.queue.max_payloads",
		"trace.writer.traces.exp_backoff_max_duration_seconds":   "apm_config.trace_writer.queue.exp_backoff_max_duration_seconds",
		"trace.writer.traces.exp_backoff_base_milliseconds":      "apm_config.trace_writer.queue.exp_backoff_base_milliseconds",
		"trace.writer.traces.exp_backoff_growth_base":            "apm_config.trace_writer.queue.exp_backoff_growth_base",
	} {
		if v, ok := agentConfig[iniKey]; ok {
			configConverter.Set(yamlKey, v)
		}
	}

	// event extraction rates
	prefix1 := "trace.analyzed_rate_by_service."
	prefix2 := "trace.analyzed_spans."
	set1 := make(map[string]string)
	set2 := make(map[string]string)
	for k, v := range agentConfig {
		switch {
		case strings.HasPrefix(k, prefix1):
			set1[strings.TrimPrefix(k, prefix1)] = v
		case strings.HasPrefix(k, prefix2):
			set2[strings.TrimPrefix(k, prefix2)] = v
		default:
			continue
		}
	}
	if len(set1) > 0 {
		configConverter.Set("apm_config.analyzed_rate_by_service", set1)
	}
	if len(set2) > 0 {
		configConverter.Set("apm_config.analyzed_spans", set2)
	}

	// ignored resources
	if v, ok := agentConfig["trace.ignore.resource"]; ok {
		r, err := splitString(v, ',')
		if err == nil {
			configConverter.Set("apm_config.ignore_resources", r)
		} else {
			fmt.Println("Ignoring value provided trace.ignore.resource because of parsing error")
		}
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

func extractURLAPIKeys(agentConfig Config, configConverter *config.LegacyConfigConverter) error {
	urls := strings.Split(agentConfig["dd_url"], ",")
	keys := strings.Split(agentConfig["api_key"], ",")

	if len(urls) != len(keys) {
		return fmt.Errorf("Invalid number of 'dd_url'/'api_key': please provide one api_key for each url")
	}

	if urls[0] != "https://app.datadoghq.com" {
		// 'dd_url' is optional in v6, so only set it if it's set to a non-default value in datadog.conf
		configConverter.Set("dd_url", urls[0])
	}

	configConverter.Set("api_key", keys[0])
	if len(urls) == 1 {
		return nil
	}

	urls = urls[1:]
	keys = keys[1:]

	additionalEndpoints := map[string][]string{}
	for idx, url := range urls {
		if url == "" || keys[idx] == "" {
			return fmt.Errorf("Found empty additional 'dd_url' or 'api_key'. Please check that you don't have any misplaced commas")
		}
		if _, ok := additionalEndpoints[url]; ok {
			additionalEndpoints[url] = append(additionalEndpoints[url], keys[idx])
		} else {
			additionalEndpoints[url] = []string{keys[idx]}
		}
	}
	configConverter.Set("additional_endpoints", additionalEndpoints)
	return nil
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

func buildHistogramPercentiles(agentConfig Config) []string {
	configList := agentConfig["histogram_percentiles"]
	var histogramPercentile []string

	if configList == "" {
		// return an empty list, not an error
		return nil
	}

	// percentiles are rounded down to 2 digits and (0:1)
	configList = strings.Replace(configList, " ", "", -1)
	result := strings.Split(configList, ",")
	for _, res := range result {
		num, err := strconv.ParseFloat(res, 64)
		if num < 1 && num > 0 && err == nil {
			fixed := strconv.FormatFloat(num, 'f', 2, 64)
			histogramPercentile = append(histogramPercentile, fixed)
		} else {
			fmt.Println("warning: ignoring invalid histogram percentile", res)
		}
	}

	return histogramPercentile
}

func splitString(s string, sep rune) ([]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	r.Comma = sep
	return r.Read()
}
