// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

	// Discovery mode requires HTTP + TLS probes to observe service-to-service
	// traffic. Force-enable them regardless of any explicit user override so
	// discovery doesn't silently produce no data.
	for _, key := range []string{
		smNS("http", "enabled"),
		smNS("tls", "native", "enabled"),
		smNS("tls", "go", "enabled"),
		smNS("tls", "istio", "enabled"),
		smNS("tls", "nodejs", "enabled"),
	} {
		if !cfg.GetBool(key) {
			log.Infof("discovery mode: enabling %s (required for service map)", key)
		}
		cfg.Set(key, true, model.SourceAgentRuntime)
	}

	// Discovery mode only needs HTTP + TLS probes for service map topology.
	// Force-disable application-level protocols regardless of explicit config,
	// to keep the eBPF surface minimal and avoid capturing data we won't use.
	for _, key := range []string{
		smNS("http2", "enabled"),
		smNS("kafka", "enabled"),
		smNS("postgres", "enabled"),
		smNS("redis", "enabled"),
	} {
		if cfg.GetBool(key) {
			log.Infof("discovery mode: disabling %s (not needed for service map)", key)
		}
		cfg.Set(key, false, model.SourceAgentRuntime)
	}
}
