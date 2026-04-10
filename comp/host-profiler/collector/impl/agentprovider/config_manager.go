// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentprovider

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// healthMetricsConfig holds configuration for the internal health metrics pipeline.
type healthMetricsConfig struct {
	Enabled bool
	Targets []string
}

// hostProfilerConfig holds host-profiler settings extracted from the Agent config.
type hostProfilerConfig struct {
	DebugVerbosity        string
	AdditionalHTTPHeaders map[string]string
	DDProfilingEnabled    bool
	DDProfilingPeriod     int
	HealthMetrics         healthMetricsConfig
}

type endpoint struct {
	site    string
	apiKeys []string
}

type configManager struct {
	endpointsTotalLength int
	endpoints            []endpoint
	config               config.Component
	hostProfilerConfig   hostProfilerConfig
}

func newConfigManager(config config.Component) configManager {
	if config == nil {
		return configManager{}
	}

	endpointsTotalLength := 0
	profilingDDURL := config.GetString("apm_config.profiling_dd_url")
	ddSite := config.GetString("site")
	apiKey := config.GetString("api_key")

	var usedSite string
	switch {
	case profilingDDURL != "":
		usedSite = configutils.ExtractSiteFromURL(profilingDDURL)
		if usedSite == "" {
			log.Warnf("Could not extract site from apm_config.profiling_dd_url %s, skipping endpoint", profilingDDURL)
		}
	case ddSite != "":
		usedSite = ddSite
	default:
		usedSite = "datadoghq.com"
	}

	profilingAdditionalEndpoints := config.GetStringMapStringSlice("apm_config.profiling_additional_endpoints")
	var endpoints []endpoint
	for endpointURL, keys := range profilingAdditionalEndpoints {
		site := configutils.ExtractSiteFromURL(endpointURL)
		if site == "" {
			log.Warnf("Could not extract site from URL %s, skipping endpoint", endpointURL)
			continue
		}

		if len(keys) == 0 {
			log.Warnf("Site %s has no API key registered, skipping endpoint", site)
			continue
		}

		endpoints = append(endpoints, endpoint{
			site:    site,
			apiKeys: keys,
		})
		endpointsTotalLength += len(keys)
	}
	log.Infof("Main site inferred from core configuration is %s", usedSite)

	// Add main endpoint if we have a valid site and API key
	if usedSite != "" && apiKey != "" {
		endpoints = append(endpoints, endpoint{site: usedSite, apiKeys: []string{apiKey}})
		endpointsTotalLength++
	} else if apiKey == "" {
		log.Warnf("No API key registered for main site %s", usedSite)
	}

	// Read hostprofiler fields from leaf keys directly. GetStringMap on the parent
	// key ("hostprofiler") returns defaults instead of env var overrides, so
	// mapstructure.Decode on the parent map silently drops env-var-set values.
	hostProfilerConfig := hostProfilerConfig{
		DebugVerbosity:        config.GetString("hostprofiler.debug.verbosity"),
		AdditionalHTTPHeaders: config.GetStringMapString("hostprofiler.additional_http_headers"),
		DDProfilingEnabled:    config.GetBool("hostprofiler.ddprofiling.enabled"),
		DDProfilingPeriod:     config.GetInt("hostprofiler.ddprofiling.period"),
		HealthMetrics: healthMetricsConfig{
			Enabled: config.GetBool("hostprofiler.health_metrics.enabled"),
			Targets: config.GetStringSlice("hostprofiler.health_metrics.targets"),
		},
	}

	return configManager{
		config:               config,
		endpoints:            endpoints,
		endpointsTotalLength: endpointsTotalLength,
		hostProfilerConfig:   hostProfilerConfig,
	}
}
