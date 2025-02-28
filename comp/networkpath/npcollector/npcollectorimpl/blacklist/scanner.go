// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package blacklist stores low-value pathtests in a cache so they don't get tracerouted again.
package blacklist

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// ScannerConfig is the configuration for the blacklist scanner
type ScannerConfig struct {
	// Enabled is whether the blacklist scanner is enabled.
	// If this is false, nothing ever gets blacklisted
	Enabled bool
	// MaxTTL is the maximum TTL for a path to be considered low value.
	// This should be a low number such as 2.
	MaxTTL int
	// OnlyPrivateSubnets prevents blacklisting public IPs when enabled.
	OnlyPrivateSubnets bool
}

// Scanner is a blacklist scanner
type Scanner struct {
	config ScannerConfig
}

// NewScanner creates a new blacklist scanner
func NewScanner(config ScannerConfig) *Scanner {
	return &Scanner{
		config: config,
	}
}

// ShouldBlacklist returns true if a network path result indicates it is low value and should be blacklisted.
// It looks for short traceroutes that only have a single reachable hop.
func (s *Scanner) ShouldBlacklist(path *payload.NetworkPath) bool {
	if !s.config.Enabled {
		return false
	}
	// shouldn't happen, but avoid crashing
	if len(path.Hops) == 0 {
		return false
	}
	// we only blacklist short traceroutes
	if len(path.Hops) > s.config.MaxTTL {
		return false
	}
	// make sure the last hop is reachable
	if !path.Hops[len(path.Hops)-1].Reachable {
		return false
	}
	// none of the intermediate hops should be reachable otherwise it is a "useful" path
	for i := range len(path.Hops) - 1 {
		if path.Hops[i].Reachable {
			return false
		}
	}

	// usually, we only want to blacklist private IPs because those tend to
	// have packet encapsulation which results in those single-hop traceroutes
	if s.config.OnlyPrivateSubnets {
		destIP := net.ParseIP(path.Destination.IPAddress)
		// if it's public, don't blacklist
		if !destIP.IsPrivate() {
			return false
		}
	}

	return true
}
