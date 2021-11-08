// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package metrics

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func buildNetworkStats(procPath string, networks map[string]string, cgs *cgroups.PIDStats) (*ContainerNetworkStats, error) {
	if len(cgs.PIDs) > 0 {
		return collectNetworkStats(procPath, cgs.PIDs[0], networks)
	}

	return nil, fmt.Errorf("no process found inside this cgroup, impossible to gather network stats")
}

// collectNetworkStats retrieves the network statistics for a given pid.
// The networks map allows to optionnaly map interface name to user-friendly
// network names. If not found in the map, the interface name is used.
func collectNetworkStats(procPath string, pid int, networks map[string]string) (*ContainerNetworkStats, error) {
	procNetFile := filepath.Join(procPath, strconv.Itoa(pid), "net", "dev")
	if !filesystem.FileExists(procNetFile) {
		log.Debugf("Unable to read %s for pid %d", procNetFile, pid)
		return nil, nil
	}
	lines, err := filesystem.ReadLines(procNetFile)
	if err != nil {
		log.Debugf("Unable to read %s for pid %d", procNetFile, pid)
		return nil, nil
	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid format for %s", procNetFile)
	}

	var totalRcvd, totalSent, totalPktRcvd, totalPktSent uint64
	ifaceStats := make(map[string]InterfaceNetStats)

	// Format:
	//
	// Inter-|   Receive                                                |  Transmit
	// face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
	// eth0:    1296      16    0    0    0     0          0         0        0       0    0    0    0     0       0          0
	// lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
	//
	for _, line := range lines[2:] {
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		iface := strings.TrimSuffix(fields[0], ":")

		var stat InterfaceNetStats
		var networkName string

		if nw, ok := networks[iface]; ok {
			networkName = nw
		} else if iface == "lo" {
			continue // Ignore loopback
		} else {
			networkName = iface
		}

		rcvd, _ := strconv.ParseUint(fields[1], 10, 64)
		totalRcvd += rcvd
		convertField(&rcvd, &stat.BytesRcvd)
		pktRcvd, _ := strconv.ParseUint(fields[2], 10, 64)
		totalPktRcvd += pktRcvd
		convertField(&pktRcvd, &stat.PacketsRcvd)
		sent, _ := strconv.ParseUint(fields[9], 10, 64)
		totalSent += sent
		convertField(&sent, &stat.BytesSent)
		pktSent, _ := strconv.ParseUint(fields[10], 10, 64)
		totalPktSent += pktSent
		convertField(&pktSent, &stat.PacketsSent)

		ifaceStats[networkName] = stat
	}

	if len(ifaceStats) > 0 {
		netStats := ContainerNetworkStats{Interfaces: ifaceStats}
		convertField(&totalRcvd, &netStats.BytesRcvd)
		convertField(&totalSent, &netStats.BytesSent)
		convertField(&totalPktRcvd, &netStats.PacketsRcvd)
		convertField(&totalPktSent, &netStats.PacketsSent)

		return &netStats, nil
	}

	return nil, nil
}

// DetectNetworkDestinations lists all the networks available
// to a given PID and parses them in NetworkInterface objects
func detectNetworkDestinations(procPath string, pid int) ([]containers.NetworkDestination, error) {
	procNetFile := filepath.Join(procPath, strconv.Itoa(pid), "net", "route")
	if !filesystem.FileExists(procNetFile) {
		return nil, fmt.Errorf("%s not found", procNetFile)
	}
	lines, err := filesystem.ReadLines(procNetFile)
	if err != nil {
		return nil, err
	}
	if len(lines) < 1 {
		return nil, fmt.Errorf("empty network file %s", procNetFile)
	}

	destinations := make([]containers.NetworkDestination, 0)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		if fields[1] == "00000000" {
			continue
		}
		dest, err := strconv.ParseUint(fields[1], 16, 32)
		if err != nil {
			log.Debugf("Cannot parse destination %q: %v", fields[1], err)
			continue
		}
		mask, err := strconv.ParseUint(fields[7], 16, 32)
		if err != nil {
			log.Debugf("Cannot parse mask %q: %v", fields[7], err)
			continue
		}
		d := containers.NetworkDestination{
			Interface: fields[0],
			Subnet:    dest,
			Mask:      mask,
		}
		destinations = append(destinations, d)
	}
	return destinations, nil
}

// DefaultGateway returns the default Docker gateway.
func defaultGateway(procPath string) (net.IP, error) {
	fields, err := defaultGatewayFields(procPath)
	if err != nil || len(fields) < 3 {
		return nil, err
	}

	ipInt, err := strconv.ParseUint(fields[2], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("unable to parse ip %s from route file: %s", fields[2], err)
	}
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, uint32(ipInt))
	return ip, nil
}

// DefaultHostIPs returns the IP addresses bound to the default network interface.
// The default network interface is the one connected to the network gateway, and it is determined
// by parsing the routing table file in the proc file system.
func defaultHostIPs(procPath string) ([]string, error) {
	fields, err := defaultGatewayFields(procPath)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("missing interface information from routing file")
	}
	iface, err := net.InterfaceByName(fields[0])
	if err != nil {
		return nil, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	ips := make([]string, len(addrs))
	for i, addr := range addrs {
		// Translate CIDR blocks into IPs, if necessary
		ips[i] = strings.Split(addr.String(), "/")[0]
	}

	return ips, nil
}

// defaultGatewayFields extracts the fields associated to the interface connected
// to the network gateway from the linux routing table. As an example, for the given file in /proc/net/routes:
//
// Iface   Destination  Gateway   Flags  RefCnt  Use  Metric  Mask      MTU  Window  IRTT
// enp0s3  00000000     0202000A  0003   0       0    0       00000000  0    0       0
// enp0s3  0002000A     00000000  0001   0       0    0       00FFFFFF  0    0       0
//
// The returned value would be ["enp0s3","00000000","0202000A","0003","0","0","0","00000000","0","0","0"]
//
func defaultGatewayFields(procPath string) ([]string, error) {
	netRouteFile := filepath.Join(procPath, "net", "route")
	f, err := os.Open(netRouteFile)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			log.Errorf("Unable to open %s: %s", netRouteFile, err)
			return nil, nil
		}
		// Unknown error types will bubble up for handling.
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 1 && fields[1] == "00000000" {
			return fields, nil
		}
	}

	return nil, fmt.Errorf("couldn't retrieve default gateway information")
}
