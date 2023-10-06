// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

func buildNetworkStats(procPath string, pids []int) (*provider.ContainerNetworkStats, error) {
	// All PIDs will (normally) produce the same stats
	for _, pid := range pids {
		stats, err := collectNetworkStats(procPath, pid)
		if err == nil {
			return stats, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			log.Debugf("Unable to get network stats for PID: %d, err: %v", pid, err)
			return nil, err
		}
	}

	return nil, fmt.Errorf("no process found inside this cgroup, impossible to gather network stats")
}

// collectNetworkStats retrieves the network statistics for a given pid.
// The networks map allows to optionnaly map interface name to user-friendly
// network names. If not found in the map, the interface name is used.
func collectNetworkStats(procPath string, pid int) (*provider.ContainerNetworkStats, error) {
	procNetFile := filepath.Join(procPath, strconv.Itoa(pid), "net", "dev")
	lines, err := filesystem.ReadLines(procNetFile)
	if err != nil {
		return nil, err
	}

	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid format for %s", procNetFile)
	}

	var totalRcvd, totalSent, totalPktRcvd, totalPktSent uint64
	ifaceStats := make(map[string]provider.InterfaceNetStats)

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

		// Skip loobback
		if iface == "lo" {
			continue
		}

		var stat provider.InterfaceNetStats
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

		ifaceStats[iface] = stat
	}

	if len(ifaceStats) == 0 {
		return nil, nil
	}

	netStats := provider.ContainerNetworkStats{
		Timestamp:  time.Now(),
		Interfaces: ifaceStats,
	}
	convertField(&totalRcvd, &netStats.BytesRcvd)
	convertField(&totalSent, &netStats.BytesSent)
	convertField(&totalPktRcvd, &netStats.PacketsRcvd)
	convertField(&totalPktSent, &netStats.PacketsSent)

	// This requires to run as ~root, that's why it's fine to silently fail
	if inode, err := systemutils.GetProcessNamespaceInode(procPath, strconv.Itoa(pid), "net"); err == nil {
		netStats.NetworkIsolationGroupID = &inode
		netStats.UsingHostNetwork = systemutils.IsProcessHostNetwork(procPath, inode)
	}

	return &netStats, nil
}
