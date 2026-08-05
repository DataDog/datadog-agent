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
	smNS("http2", "enabled"),
	smNS("tls", "native", "enabled"),
	smNS("tls", "go", "enabled"),
	smNS("tls", "istio", "enabled"),
	smNS("tls", "nodejs", "enabled"),
}

// discoveryForceDisabledProtocols lists the USM protocol flags that discovery
// mode forces off to keep the eBPF surface minimal.
var discoveryForceDisabledProtocols = []string{
	smNS("kafka", "enabled"),
	smNS("postgres", "enabled"),
	smNS("redis", "enabled"),
}

const (
	defaultDiscoveryServiceCollectionBatchSize              = 500
	defaultDiscoveryServiceCollectionMaxConsecutiveTimeouts = 5
)

func adjustDiscovery(cfg model.Config) {
	adjustDiscoveryServiceCollectionBatchSize(cfg)
	adjustDiscoveryServiceCollectionMaxConsecutiveTimeouts(cfg)

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

	// The experimental SK tracer (network_config.enable_sk_tracer) is
	// incompatible with USM — adjustNetwork unconditionally disables
	// service_monitoring_config.enabled when sk_tracer is on, which would
	// silently undo the force-enable below and leave discovery mode
	// producing no data. Detect the conflict here and disable discovery
	// with a clear warning, instead of failing silently downstream.
	if cfg.GetBool(netNS("enable_sk_tracer")) {
		log.Warn("network_config.enable_sk_tracer is set; discovery service map is disabled because " +
			"sk tracer is incompatible with USM, which discovery mode requires")
		cfg.Set(discoveryNS("service_map", "enabled"), false, model.SourceAgentRuntime)
		return
	}

	// eBPF-less mode (network_config.enable_ebpfless) is also incompatible
	// with USM — pkg/network/usm/config.CheckUSMSupported returns
	// ErrNotSupported outright, which would cause the network_tracer
	// module to fail to load with a misleading "USM unsupported" error
	// rather than a clear discovery-mode message. Bail here instead.
	if cfg.GetBool(netNS("enable_ebpfless")) {
		log.Warn("network_config.enable_ebpfless is set; discovery service map is disabled because " +
			"eBPF-less mode is incompatible with USM, which discovery mode requires")
		cfg.Set(discoveryNS("service_map", "enabled"), false, model.SourceAgentRuntime)
		return
	}

	log.Info("discovery.service_map.enabled is set; booting USM monitor in restricted mode (HTTP, HTTP/2 and TLS only)")

	// Enable USM so that newUSMMonitor starts on Linux (gated on ServiceMonitoringEnabled).
	// Windows bypasses this gate via NewWindowsMonitor, but Linux requires it.
	cfg.Set(smNS("enabled"), true, model.SourceAgentRuntime)

	// Force-enable process service inference so bare-process traffic on Linux
	// is attributed to a service. Without this, services not running under a
	// container or systemd unit would be missing from the service map. On
	// Windows this defaults to true already, so the Set is a no-op there.
	cfg.Set(spNS("process_service_inference", "enabled"), true, model.SourceAgentRuntime)

	// Force-enable USM connection rollup so ephemeral source ports collapse
	// into a single (client, server) entry. With path/method already dropped
	// from the key, this is the next-largest cardinality reducer and keeps
	// the in-memory stats map well below max_stats_buffered on busy hosts.
	cfg.Set(smNS("enable_connection_rollup"), true, model.SourceAgentRuntime)

	for _, key := range discoveryForceEnabledProtocols {
		cfg.Set(key, true, model.SourceAgentRuntime)
	}

	for _, key := range discoveryForceDisabledProtocols {
		disableConfig(cfg, key, "not needed for discovery service map")
	}
}

func adjustDiscoveryServiceCollectionBatchSize(cfg model.Config) {
	key := discoveryNS("service_collection_batch_size")
	batchSize := cfg.GetInt(key)
	if batchSize >= 0 {
		return
	}

	log.Warnf("%s cannot be negative: %d, using default value %d", key, batchSize, defaultDiscoveryServiceCollectionBatchSize)
	cfg.Set(key, defaultDiscoveryServiceCollectionBatchSize, model.SourceAgentRuntime)
}

func adjustDiscoveryServiceCollectionMaxConsecutiveTimeouts(cfg model.Config) {
	key := discoveryNS("service_collection_max_consecutive_timeouts")
	maxTimeouts := cfg.GetInt(key)
	if maxTimeouts >= 0 {
		return
	}

	log.Warnf("%s cannot be negative: %d, using default value %d", key, maxTimeouts, defaultDiscoveryServiceCollectionMaxConsecutiveTimeouts)
	cfg.Set(key, defaultDiscoveryServiceCollectionMaxConsecutiveTimeouts, model.SourceAgentRuntime)
}
