// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmdiscovery sweeps the IP ranges it is asked to scan and reports
// the devices it finds to Network Device Monitoring. It never schedules a
// check: discovery is report-only.
package ndmdiscovery

// team: network-device-monitoring-core

import "encoding/json"

// Range is one IP range to sweep. Probes maps a probe kind, such as "snmp", to
// that probe's options, and a kind that is present is a kind to scan with.
type Range struct {
	ID                 string
	Namespace          string
	CIDR               string
	IntervalSec        int
	IgnoredIPAddresses []string
	Tags               []string
	Probes             map[string]json.RawMessage
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
