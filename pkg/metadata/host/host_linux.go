// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
)

// GetNetworkInfo returns host specific network configuration.
// At this time, only information queried is the ephemeral port range
func GetNetworkInfo() (*NetworkInfo, error) {
	intpair := sysctl.NewIntPair("/proc", "net/ipv4/ip_local_port_range", 0)
	low, hi, err := intpair.Get()
	if nil != err {
		return nil, err
	}
	ni := &NetworkInfo{
		EphemeralPortStart: uint16(low),
		EphemeralPortEnd:   uint16(hi),
	}
	return ni, nil
}
