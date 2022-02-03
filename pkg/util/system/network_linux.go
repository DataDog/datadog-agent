// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package system

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NetworkRoute holds one network destination subnet and it's linked interface name
type NetworkRoute struct {
	Interface string
	Subnet    uint64
	Gateway   uint64
	Mask      uint64
}

// ParseProcessRoutes parses /proc/<pid>/net/route into a list of NetworkDestionation
// If PID is 0, it parses /proc/net/route instead
func ParseProcessRoutes(procPath string, pid int) ([]NetworkRoute, error) {
	var procNetFile string
	if pid > 0 {
		procNetFile = filepath.Join(procPath, strconv.Itoa(pid), "net", "route")
	} else {
		procNetFile = filepath.Join(procPath, "net", "route")
	}

	lines, err := filesystem.ReadLines(procNetFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read file at: %s, err: %w", procNetFile, err)
	}
	if len(lines) < 1 {
		return nil, fmt.Errorf("empty network file %s", procNetFile)
	}

	routes := make([]NetworkRoute, 0, len(lines)-1)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		dest, err := strconv.ParseUint(fields[1], 16, 32)
		if err != nil {
			log.Debugf("Cannot parse destination %q: %v", fields[1], err)
			continue
		}
		gateway, err := strconv.ParseUint(fields[2], 16, 32)
		if err != nil {
			log.Debugf("Cannot parse gateway %q: %v", fields[2], err)
			continue
		}
		mask, err := strconv.ParseUint(fields[7], 16, 32)
		if err != nil {
			log.Debugf("Cannot parse mask %q: %v", fields[7], err)
			continue
		}
		d := NetworkRoute{
			Interface: fields[0],
			Subnet:    dest,
			Gateway:   gateway,
			Mask:      mask,
		}
		routes = append(routes, d)
	}
	return routes, nil
}

// // defaultGateway returns the default Docker gateway.
// func defaultGateway(procPath string) (net.IP, error) {
// 	fields, err := defaultGatewayFields(procPath)
// 	if err != nil || len(fields) < 3 {
// 		return nil, err
// 	}

// 	ipInt, err := strconv.ParseUint(fields[2], 16, 32)
// 	if err != nil {
// 		return nil, fmt.Errorf("unable to parse ip %s from route file: %s", fields[2], err)
// 	}
// 	ip := make(net.IP, 4)
// 	binary.LittleEndian.PutUint32(ip, uint32(ipInt))
// 	return ip, nil
// }

// // defaultHostIPs returns the IP addresses bound to the default network interface.
// // The default network interface is the one connected to the network gateway, and it is determined
// // by parsing the routing table file in the proc file system.
// func defaultHostIPs(procPath string) ([]string, error) {
// 	fields, err := defaultGatewayFields(procPath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if len(fields) == 0 {
// 		return nil, fmt.Errorf("missing interface information from routing file")
// 	}
// 	iface, err := net.InterfaceByName(fields[0])
// 	if err != nil {
// 		return nil, err
// 	}

// 	addrs, err := iface.Addrs()
// 	if err != nil {
// 		return nil, err
// 	}

// 	ips := make([]string, len(addrs))
// 	for i, addr := range addrs {
// 		// Translate CIDR blocks into IPs, if necessary
// 		ips[i] = strings.Split(addr.String(), "/")[0]
// 	}

// 	return ips, nil
// }

// // defaultGatewayFields extracts the fields associated to the interface connected
// // to the network gateway from the linux routing table. As an example, for the given file in /proc/net/routes:
// //
// // Iface   Destination  Gateway   Flags  RefCnt  Use  Metric  Mask      MTU  Window  IRTT
// // enp0s3  00000000     0202000A  0003   0       0    0       00000000  0    0       0
// // enp0s3  0002000A     00000000  0001   0       0    0       00FFFFFF  0    0       0
// //
// // The returned value would be ["enp0s3","00000000","0202000A","0003","0","0","0","00000000","0","0","0"]
// func defaultGatewayFields(procPath string) ([]string, error) {
// 	netRouteFile := filepath.Join(procPath, "net", "route")
// 	f, err := os.Open(netRouteFile)
// 	if err != nil {
// 		if os.IsNotExist(err) || os.IsPermission(err) {
// 			log.Errorf("Unable to open %s: %s", netRouteFile, err)
// 			return nil, nil
// 		}
// 		// Unknown error types will bubble up for handling.
// 		return nil, err
// 	}
// 	defer f.Close()

// 	scanner := bufio.NewScanner(f)
// 	for scanner.Scan() {
// 		fields := strings.Fields(scanner.Text())
// 		if len(fields) >= 1 && fields[1] == "00000000" {
// 			return fields, nil
// 		}
// 	}

// 	return nil, fmt.Errorf("couldn't retrieve default gateway information")
// }
