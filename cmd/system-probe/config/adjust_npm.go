// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"math"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxConnsMessageBatchSize     = 1000
	maxOffsetThreshold           = 3000
	defaultMaxProcessesTracked   = 1024
	defaultMaxTrackedConnections = 65536
)

func adjustNetwork(cfg config.Config) {
	ebpflessEnabled := cfg.GetBool(netNS("enable_ebpf_less"))

	limitMaxInt(cfg, spNS("max_conns_per_message"), maxConnsMessageBatchSize)

	if cfg.GetBool(spNS("disable_tcp")) {
		cfg.Set(netNS("collect_tcp_v4"), false, model.SourceAgentRuntime)
		cfg.Set(netNS("collect_tcp_v6"), false, model.SourceAgentRuntime)
	}
	if cfg.GetBool(spNS("disable_udp")) {
		cfg.Set(netNS("collect_udp_v4"), false, model.SourceAgentRuntime)
		cfg.Set(netNS("collect_udp_v6"), false, model.SourceAgentRuntime)
	}
	if cfg.GetBool(spNS("disable_ipv6")) || !kernel.IsIPv6Enabled() {
		cfg.Set(netNS("collect_tcp_v6"), false, model.SourceAgentRuntime)
		cfg.Set(netNS("collect_udp_v6"), false, model.SourceAgentRuntime)
	}

	if runtime.GOOS == "windows" {
		validateInt(cfg, spNS("closed_connection_flush_threshold"), 0, func(v int) error {
			if v != 0 && v < 1024 {
				return fmt.Errorf("closed connection notification threshold set to invalid value %d. resetting to default", v)
			}
			return nil
		})
	}

	validateInt64(cfg, spNS("max_tracked_connections"), defaultMaxTrackedConnections, func(v int64) error {
		if v <= 0 {
			return fmt.Errorf("must be a positive value")
		}
		return nil
	})
	limitMaxInt64(cfg, spNS("max_tracked_connections"), math.MaxUint32)
	// make sure max_closed_connections_buffered is equal to max_tracked_connections,
	// if the former is not set. this helps with lowering or eliminating dropped
	// closed connections in environments with mostly short-lived connections
	validateInt64(cfg, spNS("max_closed_connections_buffered"), cfg.GetInt64(spNS("max_tracked_connections")), func(v int64) error {
		if v <= 0 {
			return fmt.Errorf("must be a positive value")
		}
		return nil
	})
	limitMaxInt64(cfg, spNS("max_closed_connections_buffered"), math.MaxUint32)

	limitMaxInt(cfg, spNS("offset_guess_threshold"), maxOffsetThreshold)

	if !cfg.GetBool(netNS("enable_root_netns")) {
		cfg.Set(spNS("enable_conntrack_all_namespaces"), false, model.SourceAgentRuntime)
	}

	validateInt(cfg, evNS("network_process", "max_processes_tracked"), defaultMaxProcessesTracked, func(v int) error {
		if v <= 0 {
			return fmt.Errorf("`%d` is 0 or less", v)
		}
		return nil
	})

	if cfg.GetBool(evNS("network_process", "enabled")) && !ProcessEventDataStreamSupported() {
		log.Warn("disabling process event monitoring as it is not supported for this kernel version")
		cfg.Set(evNS("network_process", "enabled"), false, model.SourceAgentRuntime)
	}

	// if npm connection rollups are enabled, but usm rollups are not,
	// then disable npm rollups as well
	if cfg.GetBool(netNS("enable_connection_rollup")) && !cfg.GetBool(smNS("enable_connection_rollup")) {
		log.Warn("disabling NPM connection rollups since USM connection rollups are not enabled")
		cfg.Set(netNS("enable_connection_rollup"), false, model.SourceAgentRuntime)
	}

	// disable features that are not supported on certain
	// configs/platforms
	var disableConfigs []string
	if ebpflessEnabled {
		disableConfigs = append(disableConfigs,
			spNS("enable_conntrack_all_namespaces"),
			netNS("enable_protocol_classification"),
			netNS("enable_http_monitoring"),
			netNS("enable_https_monitoring"),
			evNS("network_process", "enabled"),
			netNS("enable_root_netns"),
		)
	}

	for _, c := range disableConfigs {
		if cfg.GetBool(c) {
			log.Warnf("disabling %s since it is not supported for this config/platform", c)
			cfg.Set(c, false, model.SourceAgentRuntime)
		}
	}
}
