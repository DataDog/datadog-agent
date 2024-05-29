// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package failure

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var telemetryModuleName = "network_tracer__failure"

var failureTelemetry = struct {
	failedConnOrphans telemetry.Counter
}{
	telemetry.NewCounter(telemetryModuleName, "failed_conn_orphans", []string{}, "Counter measuring the number of orphans after associating failed connections with a closed connection"),
}

// FailedConnStats is a wrapper to help document the purpose of the underlying map
type FailedConnStats struct {
	CountByErrCode map[uint32]uint32
	Expiry         int64
}

// String returns a string representation of the failedConnStats
func (t FailedConnStats) String() string {
	return fmt.Sprintf(
		"FailedConnStats{CountByErrCode: %v, Expiry: %d}", t.CountByErrCode, t.Expiry,
	)
}

// FailedConnMap is a map of connection tuples to failed connection stats
type FailedConnMap map[ebpf.ConnTuple]*FailedConnStats

// FailedConns is a struct to hold failed connections
type FailedConns struct {
	FailedConnMap map[ebpf.ConnTuple]*FailedConnStats
	sync.RWMutex
}

// NewFailedConns returns a new FailedConns struct
func NewFailedConns() *FailedConns {
	return &FailedConns{
		FailedConnMap: make(map[ebpf.ConnTuple]*FailedConnStats),
	}
}

// MatchFailedConn increments the failed connection counters for a given connection based on the failed connection map
func (fc *FailedConns) MatchFailedConn(conn *network.ConnectionStats) {
	if conn.Type != network.TCP {
		return
	}
	connTuple := connStatsToTuple(conn)

	// Read lock to check if the connection exists
	fc.RLock()
	failedConn, ok := fc.FailedConnMap[connTuple]
	fc.RUnlock()

	// If connection exists, proceed to increment failure count and delete
	if ok {
		conn.TCPFailures = make(map[uint32]uint32)

		// Write lock to modify the map
		for errCode, count := range failedConn.CountByErrCode {
			conn.TCPFailures[errCode] += count
		}
	}
}

// RemoveExpired removes expired failed connections from the map
func (fc *FailedConns) RemoveExpired() {
	if fc == nil {
		return
	}
	fc.Lock()
	defer fc.Unlock()

	now := time.Now().Unix()
	removed := 0

	for connTuple, failedConn := range fc.FailedConnMap {
		if failedConn.Expiry < now {
			removed++
			delete(fc.FailedConnMap, connTuple)
		}
	}

	failureTelemetry.failedConnOrphans.Add(float64(removed))
}

// connStatsToTuple converts a ConnectionStats to a ConnTuple
func connStatsToTuple(c *network.ConnectionStats) ebpf.ConnTuple {
	var tup ebpf.ConnTuple
	tup.Sport = c.SPort
	tup.Dport = c.DPort
	tup.Netns = c.NetNS
	tup.Pid = c.Pid
	if c.Family == network.AFINET {
		tup.SetFamily(ebpf.IPv4)
	} else {
		tup.SetFamily(ebpf.IPv6)
	}
	if c.Type == network.TCP {
		tup.SetType(ebpf.TCP)
	} else {
		tup.SetType(ebpf.UDP)
	}
	if c.Source.IsZero() {
		tup.Saddr_l, tup.Saddr_h = 0, 0
	} else {
		tup.Saddr_l, tup.Saddr_h = util.ToLowHigh(c.Source)
	}
	if c.Dest.IsZero() {
		tup.Daddr_l, tup.Daddr_h = 0, 0
	} else {
		tup.Daddr_l, tup.Daddr_h = util.ToLowHigh(c.Dest)
	}
	return tup
}
