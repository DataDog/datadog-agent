// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// loadEnv loads all known environment variables into the global config.
func loadEnv() {
	// Warning: do not use BindEnv to bind config variables. They will be overridden
	// when using the legacy config loader.
	for _, override := range []struct{ env, key string }{
		// Core agent:
		{"DD_SITE", "site"},
		{"DD_API_KEY", "api_key"},
		{"DD_HOSTNAME", "hostname"},
		{"DD_BIND_HOST", "bind_host"},
		{"DD_DOGSTATSD_PORT", "dogstatsd_port"},
		{"DD_LOG_LEVEL", "log_level"},
		{"HTTPS_PROXY", "proxy.https"}, // deprecated
		{"DD_PROXY_HTTPS", "proxy.https"},

		// APM specific:
		{"DD_CONNECTION_LIMIT", "apm_config.connection_limit"}, // deprecated
		{"DD_APM_CONNECTION_LIMIT", "apm_config.connection_limit"},
		{"DD_APM_ENABLED", "apm_config.enabled"},
		{"DD_APM_ENV", "apm_config.env"},
		{"DD_APM_NON_LOCAL_TRAFFIC", "apm_config.apm_non_local_traffic"},
		{"DD_APM_DD_URL", "apm_config.apm_dd_url"},
		{"DD_APM_CONNECTION_RESET_INTERVAL", "apm_config.connection_reset_interval"},
		{"DD_RECEIVER_PORT", "apm_config.receiver_port"}, // deprecated
		{"DD_APM_RECEIVER_PORT", "apm_config.receiver_port"},
		{"DD_MAX_EPS", "apm_config.max_events_per_second"}, // deprecated
		{"DD_APM_MAX_EPS", "apm_config.max_events_per_second"},
		{"DD_MAX_TPS", "apm_config.max_traces_per_second"}, // deprecated
		{"DD_APM_MAX_TPS", "apm_config.max_traces_per_second"},
		{"DD_APM_MAX_MEMORY", "apm_config.max_memory"},
		{"DD_APM_MAX_CPU_PERCENT", "apm_config.max_cpu_percent"},
		{"DD_APM_RECEIVER_SOCKET", "apm_config.receiver_socket"},
	} {
		if v := os.Getenv(override.env); v != "" {
			config.Datadog.Set(override.key, v)
		}
	}
	for _, envKey := range []string{
		"DD_IGNORE_RESOURCE", // deprecated
		"DD_APM_IGNORE_RESOURCES",
	} {
		if v := os.Getenv(envKey); v != "" {
			if r, err := splitString(v, ','); err != nil {
				log.Warnf("%q value not loaded: %v", envKey, err)
			} else {
				config.Datadog.Set("apm_config.ignore_resources", r)
			}
		}
	}
	if v := os.Getenv("DD_APM_ANALYZED_SPANS"); v != "" {
		analyzedSpans, err := parseAnalyzedSpans(v)
		if err == nil {
			config.Datadog.Set("apm_config.analyzed_spans", analyzedSpans)
		} else {
			log.Errorf("Bad format for %s it should be of the form \"service_name|operation_name=rate,other_service|other_operation=rate\", error: %v", "DD_APM_ANALYZED_SPANS", err)
		}
	}
	if v := os.Getenv("DD_APM_REPLACE_TAGS"); v != "" {
		replaceTags, err := parseReplaceTags(v)
		if err == nil {
			config.Datadog.Set("apm_config.replace_tags", replaceTags)
		} else {
			log.Errorf("Bad format for %s it should be of the form '[{\"name\": \"tag_name\",\"pattern\":\"pattern\",\"repl\":\"replace_str\"}]', error: %v", "DD_APM_REPLACE_TAGS", err)
		}
	}
	if v := os.Getenv("DD_APM_ADDITIONAL_ENDPOINTS"); v != "" {
		ap := make(map[string][]string)
		if err := json.Unmarshal([]byte(v), &ap); err != nil {
			log.Errorf(`Could not parse DD_APM_ADDITIONAL_ENDPOINTS: %v. It must be of the form '{"https://trace.agent.datadoghq.com": ["apikey1", ...], ...}'.`, err)
		} else {
			config.Datadog.Set("apm_config.additional_endpoints", ap)
		}
	}
}

func parseNameAndRate(token string) (string, float64, error) {
	parts := strings.Split(token, "=")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("Bad format")
	}
	rate, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", 0, fmt.Errorf("Unabled to parse rate")
	}
	return parts[0], rate, nil
}

// parseAnalyzedSpans parses the env string to extract a map of spans to be analyzed by service and operation.
// the format is: service_name|operation_name=rate,other_service|other_operation=rate
func parseAnalyzedSpans(env string) (analyzedSpans map[string]float64, err error) {
	analyzedSpans = make(map[string]float64)
	if env == "" {
		return
	}
	tokens := strings.Split(env, ",")
	for _, token := range tokens {
		name, rate, err := parseNameAndRate(token)
		if err != nil {
			return nil, err
		}
		analyzedSpans[name] = rate
	}
	return
}

func parseReplaceTags(env string) ([]map[string]string, error) {
	var replaceTags []map[string]string
	err := json.Unmarshal([]byte(env), &replaceTags)
	if err != nil {
		return nil, err
	}
	return replaceTags, nil
}
