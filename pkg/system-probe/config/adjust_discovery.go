// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// discoveryForceEnabledProtocols lists the USM protocol flags that discovery
// mode forces on so the monitor never silently produces no data.
var discoveryForceEnabledProtocols = []string{
	smNS("http", "enabled"),
	smNS("tls", "native", "enabled"),
	smNS("tls", "go", "enabled"),
	smNS("tls", "istio", "enabled"),
	smNS("tls", "nodejs", "enabled"),
}

// discoveryForceDisabledProtocols lists the USM protocol flags that discovery
// mode forces off to keep the eBPF surface minimal.
var discoveryForceDisabledProtocols = []string{
	smNS("http2", "enabled"),
	smNS("kafka", "enabled"),
	smNS("postgres", "enabled"),
	smNS("redis", "enabled"),
}

func adjustDiscovery(cfg model.Config) {
	if !cfg.GetBool(discoveryNS("service_map", "enabled")) {
		return
	}

	// If paid USM is also enabled, discovery mode is redundant — USM provides
	// a strict superset of the data. Disable discovery to avoid duplicate work
	// and ambiguous billing signals.
	if cfg.GetBool(smNS("enabled")) {
		log.Warn("both service_monitoring_config.enabled and discovery.service_map.enabled are set; " +
			"discovery mode is ignored when full USM is enabled")
		cfg.Set(discoveryNS("service_map", "enabled"), false, model.SourceAgentRuntime)
		return
	}

	log.Info("discovery.service_map.enabled is set; booting USM monitor in restricted mode (HTTP only, no billing)")

	// Enable USM so that newUSMMonitor starts on Linux (gated on ServiceMonitoringEnabled).
	// Windows bypasses this gate via NewWindowsMonitor, but Linux requires it.
	cfg.Set(smNS("enabled"), true, model.SourceAgentRuntime)

	for _, key := range discoveryForceEnabledProtocols {
		cfg.Set(key, true, model.SourceAgentRuntime)
	}

	for _, key := range discoveryForceDisabledProtocols {
		disableConfig(cfg, key, "not needed for discovery service map")
	}
}
