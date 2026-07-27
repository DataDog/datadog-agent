// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import "time"

// DefaultMaxTTL is the default maximum number of hops for Network Path
// traceroute tests.
const DefaultMaxTTL = 30

// tracerouteExecutionBudgetRatio allocates 90% of the total timeout to per-hop
// probes. The remaining 10% leaves time for traceroute-library overhead and
// for the call to return before the total test deadline expires.
const tracerouteExecutionBudgetRatio = 0.9

// PerHopTimeout converts a total traceroute execution budget into a timeout
// for each hop while preserving the return-path buffer described by
// tracerouteExecutionBudgetRatio.
// maxTTL must be greater than zero.
func PerHopTimeout(totalTimeout time.Duration, maxTTL uint8) time.Duration {
	return time.Duration(float64(totalTimeout) * tracerouteExecutionBudgetRatio / float64(maxTTL))
}
