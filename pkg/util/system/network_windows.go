// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package system

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil/iphelper"

	"golang.org/x/sys/windows"
)

// ParseProcessRoutes uses routing table
func ParseProcessRoutes(procPath string, pid int) ([]NetworkRoute, error) {
	// TODO: Filter by PID
	routingTable, err := iphelper.GetIPv4RouteTable()
	if err != nil {
		return nil, err
	}
	interfaceTable, err := iphelper.GetIFTable()
	if err != nil {
		return nil, err
	}
	netDestinations := make([]NetworkRoute, len(routingTable))
	for _, row := range routingTable {
		itf := interfaceTable[row.DwForwardIfIndex]
		netDest := NetworkRoute{
			Interface: windows.UTF16ToString(itf.WszName[:]),
			Subnet:    uint64(row.DwForwardDest),
			Mask:      uint64(row.DwForwardMask),
			Gateway:   uint64(row.DwForwardNextHop),
		}
		netDestinations = append(netDestinations, netDest)
	}
	return netDestinations, nil
}
