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
	Target  string
}

// ddProfilingConfig holds configuration for the ddprofiling self-profiling extension.
type ddProfilingConfig struct {
	Enabled bool
	Period  int
	// Port is the local port the ddprofiling HTTP server listens on. 0 means the
	// extension applies its own default.
	Port int
}

// hpFlareConfig holds configuration for the hpflare diagnostics extension.
type hpFlareConfig struct {
	Port int
}

// hostProfilerConfig holds host-profiler settings extracted from the Agent config.
type hostProfilerConfig struct {
	DebugVerbosity        string
	AdditionalHTTPHeaders map[string]string
	DDProfiling           ddProfilingConfig
	HeapProfiling         bool
	LiveHeapProfiling     bool
	Tracers               string
	HealthMetrics         healthMetricsConfig
	HPFlare               hpFlareConfig
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
	profilingSendToMainEndpoint := config.GetBool("apm_config.profiling_send_to_main_endpoint")
	apiKey := config.GetString("api_key")

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
	if profilingSendToMainEndpoint {
		usedSite := resolveMainProfilingSite(config)
		if usedSite != "" && apiKey != "" {
			endpoints = append(endpoints, endpoint{site: usedSite, apiKeys: []string{apiKey}})
			endpointsTotalLength++
		} else if apiKey == "" {
			log.Warnf("No API key registered for main site %s", usedSite)
		} else {
			log.Warnf("Skipping main profiling endpoint: could not determine site")
		}
	}

	// Read hostprofiler fields from leaf keys directly. GetStringMap on the parent
	// key ("hostprofiler") returns defaults instead of env var overrides, so
	// mapstructure.Decode on the parent map silently drops env-var-set values.
	hostProfilerConfig := hostProfilerConfig{
		DebugVerbosity:        config.GetString("hostprofiler.debug.verbosity"),
		AdditionalHTTPHeaders: config.GetStringMapString("hostprofiler.additional_http_headers"),
		DDProfiling: ddProfilingConfig{
			Enabled: config.GetBool("hostprofiler.ddprofiling.enabled"),
			Period:  config.GetInt("hostprofiler.ddprofiling.period"),
			Port:    config.GetInt("hostprofiler.ddprofiling.port"),
		},
		HeapProfiling:     config.GetBool("hostprofiler.heap_profiling"),
		LiveHeapProfiling: config.GetBool("hostprofiler.live_heap_profiling"),
		Tracers:           config.GetString("hostprofiler.tracers"),
		HealthMetrics: healthMetricsConfig{
			Enabled: config.GetBool("hostprofiler.health_metrics.enabled"),
			Target:  config.GetString("hostprofiler.health_metrics.target"),
		},
		HPFlare: hpFlareConfig{
			Port: config.GetInt("hostprofiler.hpflare.port"),
		},
	}

	return configManager{
		config:               config,
		endpoints:            endpoints,
		endpointsTotalLength: endpointsTotalLength,
		hostProfilerConfig:   hostProfilerConfig,
	}
}

// resolveMainProfilingSite returns the site to use for the main profiling
// endpoint, preferring an explicit DD URL override, then the configured site,
// then falling back to datadoghq.com.
func resolveMainProfilingSite(config config.Component) string {
	if ddURL := config.GetString("apm_config.profiling_dd_url"); ddURL != "" {
		site := configutils.ExtractSiteFromURL(ddURL)
		if site == "" {
			log.Warnf("Could not extract site from apm_config.profiling_dd_url %s, skipping endpoint", ddURL)
		}
		return site
	}
	if site := config.GetString("site"); site != "" {
		return site
	}
	return "datadoghq.com"
}
