// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var (
	ephemeralLow  = uint16(0)
	ephemeralHigh = uint16(0)

	initEphemeralIntPair sync.Once
	ephemeralIntPair     *sysctl.IntPair
)

// IsPortInEphemeralRange returns whether the port is ephemeral based on the OS-specific configuration.
func IsPortInEphemeralRange(p uint16) EphemeralPortType {
	initEphemeralIntPair.Do(func() {
		ephemeralIntPair = sysctl.NewIntPair(util.GetProcRoot(), "net/ipv4/ip_local_port_range", time.Hour)
		low, hi, err := ephemeralIntPair.Get()
		if err == nil {
			if low > 0 && low <= math.MaxUint16 {
				ephemeralLow = uint16(low)
			}
			if hi > 0 && hi <= math.MaxUint16 {
				ephemeralHigh = uint16(hi)
			}
		}
	})
	if ephemeralLow == 0 || ephemeralHigh == 0 {
		return EphemeralUnknown
	}
	if p >= ephemeralLow && p <= ephemeralHigh {
		return EphemeralTrue
	}
	return EphemeralFalse
}
