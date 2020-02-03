// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"fmt"
	"strconv"
	"strings"

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
