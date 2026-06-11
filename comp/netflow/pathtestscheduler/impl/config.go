// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package impl contains the implementation of the pathtestscheduler component.
package impl

import (
	"fmt"
	"net/netip"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// defaultMaxDestinationsPerFlush is the default cap on converted connections
// per flush if the config key is unset or zero.
const defaultMaxDestinationsPerFlush = 50

// schedulerConfig holds the configuration for the NDM dynamic path test scheduler.
// All settings are loaded from network_path.netflow_monitoring.*.
type schedulerConfig struct {
	// enabled is the master switch. Off by default for safe rollout.
	enabled bool

	// maxDestinationsPerFlush caps the number of converted connections handed to
	// npcollector per aggregator flush. Excess connections are dropped and reported
	// via the max_destinations_cap drop reason metric.
	maxDestinationsPerFlush int

	// destExcludes is a list of CIDR prefixes whose destinations are dropped
	// before being handed to npcollector. Bad CIDRs are surfaced at load time.
	destExcludes []string

	// destExcludePrefixes is the parsed form of destExcludes. Parsed at config
	// load time so invalid CIDRs are detected early.
	destExcludePrefixes []netip.Prefix

	// minPackets, when > 0, skips flows with fewer than this many packets.
	// NOTE: the converter currently does not carry per-flow packets/bytes through
	// to NetworkPathConnection (the aggregated connection has no packet/byte count
	// field). min_packets and min_bytes filtering is therefore a known limitation
	// of the current MVP — filter values are loaded but not applied. A follow-up
	// task should add packet/byte counts to NetworkPathConnection if needed.
	minPackets int

	// minBytes, when > 0, skips flows with fewer than this many bytes.
	// See the note on minPackets above.
	minBytes int
}

// newSchedulerConfig loads the pathtestscheduler config from the agent config component.
// Returns an error if any CIDR in dest_excludes fails to parse.
func newSchedulerConfig(agentConfig config.Component) (*schedulerConfig, error) {
	maxDest := agentConfig.GetInt("network_path.netflow_monitoring.max_destinations_per_flush")
	if maxDest <= 0 {
		maxDest = defaultMaxDestinationsPerFlush
	}

	cfg := &schedulerConfig{
		enabled:                 agentConfig.GetBool("network_path.netflow_monitoring.enabled"),
		maxDestinationsPerFlush: maxDest,
		destExcludes:            agentConfig.GetStringSlice("network_path.netflow_monitoring.dest_excludes"),
		minPackets:              agentConfig.GetInt("network_path.netflow_monitoring.min_packets"),
		minBytes:                agentConfig.GetInt("network_path.netflow_monitoring.min_bytes"),
	}

	// Parse CIDR exclusion list at load time so bad values surface immediately.
	cfg.destExcludePrefixes = make([]netip.Prefix, 0, len(cfg.destExcludes))
	for _, cidr := range cfg.destExcludes {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR in network_path.netflow_monitoring.dest_excludes: %q: %w", cidr, err)
		}
		cfg.destExcludePrefixes = append(cfg.destExcludePrefixes, prefix)
	}

	return cfg, nil
}
