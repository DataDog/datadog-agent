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

// ParseProcessIPs parses /proc/<pid>/net/fib_trie and returns the /32 IP
// addresses found. The result does not contain duplicate IPs.
//
// Here's an example of /proc/<pid>/net/fib_trie that shows its format:
//	Main:
//	  +-- 0.0.0.0/1 2 0 2
//	     +-- 0.0.0.0/4 2 0 2
//	        |-- 0.0.0.0
//	           /0 universe UNICAST
//	        +-- 10.4.0.0/24 2 1 2
//	           |-- 10.4.0.0
//	              /32 link BROADCAST
//	              /24 link UNICAST
//	           +-- 10.4.0.192/26 2 0 2
//	              |-- 10.4.0.216
//	                 /32 host LOCAL
//	              |-- 10.4.0.255
//	                 /32 link BROADCAST
//	     +-- 127.0.0.0/8 2 0 2
//	        +-- 127.0.0.0/31 1 0 0
//	           |-- 127.0.0.0
//	              /32 link BROADCAST
//	              /8 host LOCAL
//	           |-- 127.0.0.1
//	              /32 host LOCAL
//	        |-- 127.255.255.255
//	           /32 link BROADCAST
//	Local:
//	  +-- 0.0.0.0/1 2 0 2
//	     +-- 0.0.0.0/4 2 0 2
//	        |-- 0.0.0.0
//	           /0 universe UNICAST
//	        +-- 10.4.0.0/24 2 1 2
//	           |-- 10.4.0.0
//	              /32 link BROADCAST
//	              /24 link UNICAST
//	           +-- 10.4.0.192/26 2 0 2
//	              |-- 10.4.0.216
//	                 /32 host LOCAL
//	              |-- 10.4.0.255
//	                 /32 link BROADCAST
//	     +-- 127.0.0.0/8 2 0 2
//	        +-- 127.0.0.0/31 1 0 0
//	           |-- 127.0.0.0
//	              /32 link BROADCAST
//	              /8 host LOCAL
//	           |-- 127.0.0.1
//	              /32 host LOCAL
//	        |-- 127.255.255.255
//	           /32 link BROADCAST
//
// The IPs that we're interested in are the ones that appear above lines that
// contain "/32 host".
func ParseProcessIPs(procPath string, pid int, filterFunc func(string) bool) ([]string, error) {
	var procNetFibTrieFile string
	if pid > 0 {
		procNetFibTrieFile = filepath.Join(procPath, strconv.Itoa(pid), "net", "fib_trie")
	} else {
		procNetFibTrieFile = filepath.Join(procPath, "net", "fib_trie")
	}

	lines, err := filesystem.ReadLines(procNetFibTrieFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read file at: %s, err: %w", procNetFibTrieFile, err)
	}
	if len(lines) < 1 {
		return nil, fmt.Errorf("empty network file %s", procNetFibTrieFile)
	}

	IPs := make(map[string]bool)
	for i, line := range lines {
		if strings.Contains(line, "/32 host") && i > 0 {
			split := strings.Split(lines[i-1], "|-- ")
			if len(split) == 2 {
				ip := split[1]
				if filterFunc == nil || filterFunc(ip) {
					IPs[ip] = true
				}
			}
		}
	}

	var uniqueIPs []string
	for IP := range IPs {
		uniqueIPs = append(uniqueIPs, IP)
	}

	return uniqueIPs, nil
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
