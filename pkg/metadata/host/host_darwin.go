// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/shirou/gopsutil/host"
)

type osVersion [3]interface{}

const osName = runtime.GOOS

func fillOsVersion(stats *systemStats, info *host.InfoStat) {
	stats.Macver = osVersion{info.PlatformVersion, [3]string{"", "", ""}, runtime.GOARCH}
}

// GetNetworkInfo returns host specific network configuration.
// At this time, only information queried is the ephemeral port range
func GetNetworkInfo() (*NetworkInfo, error) {
	rangestart := sysctl.NewInt16("net.inet.ip.portrange.first", 0)
	rangeend := sysctl.NewInt16("net.inet.ip.portrange.last", 0)

	low, err := rangestart.Get()
	if nil != err {
		return nil, err
	}
	hi, err := rangeend.Get()
	if nil != err {
		return nil, err
	}
	ni := &NetworkInfo{
		EphemeralPortStart: low,
		EphemeralPortEnd:   hi,
	}
	return ni, nil
}
