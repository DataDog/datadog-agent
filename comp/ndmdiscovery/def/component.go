// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmdiscovery sweeps the IP ranges it is asked to scan and reports
// the devices it finds to Network Device Monitoring. It never schedules a
// check: discovery is report-only.
package ndmdiscovery

// team: network-device-monitoring-core

// SNMPOptions are the per-range SNMP probe knobs. A nil Retries means the
// Agent default, which an explicit zero overrides.
type SNMPOptions struct {
	Port      int
	TimeoutMs int
	Retries   *int
}

// PingOptions are the per-range ICMP probe knobs.
type PingOptions struct {
	Count      int
	IntervalMs int
	TimeoutMs  int
}

// Range is one IP range to sweep. A nil PingOptions disables ping for the
// range.
type Range struct {
	ID                 string
	Namespace          string
	CIDR               string
	CredentialIDs      []string
	IntervalSec        int
	IgnoredIPAddresses []string
	Tags               []string
	SNMPOptions        *SNMPOptions
	PingOptions        *PingOptions
}

// Component is the component type.
type Component interface {
	// Schedule makes the given ranges the complete set of swept ranges: a
	// range that is active but absent from the argument is stopped. It returns
	// one error per rejected range, keyed by range id.
	Schedule(ranges []Range) map[string]error
	// RangeCount is the number of ranges currently swept.
	RangeCount() int
}
