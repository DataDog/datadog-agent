// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package failure

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FailedConnStats is a wrapper to help document the purpose of the underlying map
type FailedConnStats struct {
	CountByErrCode map[uint32]uint32
}

// String returns a string representation of the failedConnStats
func (t FailedConnStats) String() string {
	return fmt.Sprintf(
		"failedConns: {countByErrCode: %+v}", t.CountByErrCode,
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
func MatchFailedConn(conn *network.ConnectionStats, failedConnMap *FailedConns) {
	if conn.Type != network.TCP {
		return
	}
	connTuple := connStatsToTuple(conn)
	failedConnMap.RLock()
	defer failedConnMap.RUnlock()
	log.Errorf("connTuple: %+v", conn)
	log.Errorf("failedConnMap: %+v", failedConnMap.FailedConnMap)
	log.Errorf("")
	if failedConn, ok := failedConnMap.FailedConnMap[connTuple]; ok {
		conn.TCPFailures = make(map[uint32]uint32)
		for errCode, count := range failedConn.CountByErrCode {
			// TODO: delete entry from map if we find a match so we don't match the same failure to different conns
			conn.TCPFailures[errCode] += count
		}
	}
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
