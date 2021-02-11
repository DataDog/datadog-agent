// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package cgroup

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// collectNetworkStats retrieves the network statistics for a given pid.
// The networks map allows to optionnaly map interface name to user-friendly
// network names. If not found in the map, the interface name is used.
func collectNetworkStats(pid int, networks map[string]string) (metrics.ContainerNetStats, error) {
	netStats := metrics.ContainerNetStats{}

	procNetFile := hostProc(strconv.Itoa(pid), "net", "dev")
	if !pathExists(procNetFile) {
		log.Debugf("Unable to read %s for pid %d", procNetFile, pid)
		return netStats, nil
	}
	lines, err := readLines(procNetFile)
	if err != nil {
		log.Debugf("Unable to read %s for pid %d", procNetFile, pid)
		return netStats, nil
	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid format for %s", procNetFile)
	}

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
		iface := fields[0][:len(fields[0])-1]

		var stat *metrics.InterfaceNetStats

		if nw, ok := networks[iface]; ok {
			stat = &metrics.InterfaceNetStats{NetworkName: nw}
		} else if iface == "lo" {
			continue // Ignore loopback
		} else {
			stat = &metrics.InterfaceNetStats{NetworkName: iface}
		}

		rcvd, _ := strconv.Atoi(fields[1])
		stat.BytesRcvd = uint64(rcvd)
		pktRcvd, _ := strconv.Atoi(fields[2])
		stat.PacketsRcvd = uint64(pktRcvd)
		sent, _ := strconv.Atoi(fields[9])
		stat.BytesSent = uint64(sent)
		pktSent, _ := strconv.Atoi(fields[10])
		stat.PacketsSent = uint64(pktSent)

		netStats = append(netStats, stat)
	}
	return netStats, nil
}

// DetectNetworkDestinations lists all the networks available
// to a given PID and parses them in NetworkInterface objects
func detectNetworkDestinations(pid int) ([]containers.NetworkDestination, error) {
	procNetFile := hostProc(strconv.Itoa(pid), "net", "route")
	if !pathExists(procNetFile) {
		return nil, fmt.Errorf("%s not found", procNetFile)
	}
	lines, err := readLines(procNetFile)
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
func defaultGateway() (net.IP, error) {
	fields, err := defaultGatewayFields()
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
func defaultHostIPs() ([]string, error) {
	fields, err := defaultGatewayFields()
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
func defaultGatewayFields() ([]string, error) {
	procRoot := config.Datadog.GetString("proc_root")
	netRouteFile := filepath.Join(procRoot, "net", "route")
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
