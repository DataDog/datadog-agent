// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"fmt"
	"strconv"
	"strings"

	log "github.com/cihub/seelog"
)

// SumInterfaces sums stats from all interfaces into a single InterfaceNetStats
func (ns ContainerNetStats) SumInterfaces() *InterfaceNetStats {
	sum := &InterfaceNetStats{}
	for _, stat := range ns {
		sum.BytesSent += stat.BytesSent
		sum.BytesRcvd += stat.BytesRcvd
		sum.PacketsSent += stat.PacketsSent
		sum.PacketsRcvd += stat.PacketsRcvd
	}
	return sum
}

func collectNetworkStats(containerID string, pid int, networks []dockerNetwork) (ContainerNetStats, error) {
	netStats := ContainerNetStats{}

	procNetFile := hostProc(strconv.Itoa(int(pid)), "net", "dev")
	if !pathExists(procNetFile) {
		log.Debugf("Unable to read %s for container %s", procNetFile, containerID)
		return netStats, nil
	}
	lines, err := readLines(procNetFile)
	if err != nil {
		log.Debugf("Unable to read %s for container %s", procNetFile, containerID)
		return netStats, nil
	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid format for %s", procNetFile)
	}

	nwByIface := make(map[string]dockerNetwork)
	for _, nw := range networks {
		nwByIface[nw.iface] = nw
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

		if nw, ok := nwByIface[iface]; ok {
			stat := &InterfaceNetStats{NetworkName: nw.dockerName}
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
	}
	return netStats, nil
}
