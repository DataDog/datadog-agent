// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package icmp

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// NotSupportedError
type NotSupportedError struct {
	Err error
}

func (e *NotSupportedError) Error() string {
	return fmt.Sprintf("ICMP not supported by the target: %s", e.Err)
}
func (e *NotSupportedError) Unwrap() error {
	return e.Err
}

// Params is the icmp traceroute parameters
type Params struct {
	Target         netip.Addr
	ParallelParams common.TracerouteParallelParams
}

func (p Params) validate() error {
	if !p.Target.IsValid() {
		return fmt.Errorf("icmp traceroute provided invalid IP address")
	}
	//if p.Target.Is6() {
	//	return fmt.Errorf("icmp traceroute does not support IPv6 yet")
	//}
	return nil
}

func (p Params) Timeout() time.Duration {
	return time.Duration(float64(p.ParallelParams.MaxTimeout()) * 1.10)
}
