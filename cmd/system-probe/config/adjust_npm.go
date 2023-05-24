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
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	maxConnsMessageBatchSize     = 1000
	maxOffsetThreshold           = 3000
	defaultMaxProcessesTracked   = 1024
	defaultMaxTrackedConnections = 65536
)

func adjustNetwork(cfg config.Config) {
	limitMaxInt(cfg, spNS("max_conns_per_message"), maxConnsMessageBatchSize)

	if cfg.GetBool(spNS("disable_tcp")) {
		cfg.Set(netNS("collect_tcp_v4"), false)
		cfg.Set(netNS("collect_tcp_v6"), false)
	}
	if cfg.GetBool(spNS("disable_udp")) {
		cfg.Set(netNS("collect_udp_v4"), false)
		cfg.Set(netNS("collect_udp_v6"), false)
	}
	if cfg.GetBool(spNS("disable_ipv6")) || !kernel.IsIPv6Enabled() {
		cfg.Set(netNS("collect_tcp_v6"), false)
		cfg.Set(netNS("collect_udp_v6"), false)
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
		cfg.Set(spNS("enable_conntrack_all_namespaces"), false)
	}

	validateInt(cfg, evNS("network_process", "max_processes_tracked"), defaultMaxProcessesTracked, func(v int) error {
		if v <= 0 {
			return fmt.Errorf("`%d` is 0 or less", v)
		}
		return nil
	})
}
