// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package icmp has icmp tracerouting logic
package icmp

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// NotSupportedError means not on a supported platform.
type NotSupportedError struct {
	Err error
}

func (e *NotSupportedError) Error() string {
	return fmt.Sprintf("icmp not supported by the target: %s", e.Err)
}
func (e *NotSupportedError) Unwrap() error {
	return e.Err
}

// Params is the ICMP traceroute parameters
type Params struct {
	// Target is the IP:port to traceroute
	Target netip.Addr
	// ParallelParams are the standard params for parallel traceroutes
	ParallelParams common.TracerouteParallelParams
}

// MaxTimeout returns the sum of all timeouts/delays for an ICMP traceroute
func (p Params) MaxTimeout() time.Duration {
	ttl := time.Duration(p.ParallelParams.MaxTTL - p.ParallelParams.MinTTL)
	return ttl * p.ParallelParams.MaxTimeout()
}
