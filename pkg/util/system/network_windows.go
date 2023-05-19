// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package system

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"

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

// GetDefaultGateway returns the default gateway used by container implementation
func GetDefaultGateway(procPath string) (net.IP, error) {
	fields, err := defaultGatewayFields()
	if err != nil {
		return nil, err
	}
	return net.ParseIP(fields[2]), nil
}

// Output from route print 0.0.0.0:
//
// λ route print 0.0.0.0
// ===========================================================================
// Interface List
// 17...00 1c 42 86 10 92 ......Intel(R) 82574L Gigabit Network Connection
// 16...bc 9a 78 56 34 12 ......Bluetooth Device (Personal Area Network)
//
//	1...........................Software Loopback Interface 1
//
// 24...00 15 5d 2c 6f c0 ......Hyper-V Virtual Ethernet Adapter #2
// ===========================================================================
//
// IPv4 Route Table
// ===========================================================================
// Active Routes:
// Network Destination        Netmask          Gateway       Interface  Metric
//
//	0.0.0.0          0.0.0.0      10.211.55.1      10.211.55.4     25
//
// ===========================================================================
// Persistent Routes:
//
//	Network Address          Netmask  Gateway Address  Metric
//	        0.0.0.0          0.0.0.0      172.21.96.1  Default
//
// ===========================================================================
//
// IPv6 Route Table
// ===========================================================================
// Active Routes:
//
//	None
//
// Persistent Routes:
//
//	None
//
// We are interested in the Gateway and Interface fields of the Active Routes,
// so this method returns any line that has 5 fields with the first one being
// 0.0.0.0
func defaultGatewayFields() ([]string, error) {
	routeCmd := exec.Command("route", "print", "0.0.0.0")
	routeCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 5 && fields[0] == "0.0.0.0" {
			return fields, nil
		}
	}
	return nil, fmt.Errorf("couldn't retrieve default gateway information")
}
