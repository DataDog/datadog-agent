// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package failure

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	allowListErrs = map[uint32]struct{}{
		ebpf.TCPFailureConnReset:   {}, // Connection reset by peer
		ebpf.TCPFailureConnTimeout: {}, // Connection timed out
		ebpf.TCPFailureConnRefused: {}, // Connection refused
	}

	telemetryModuleName = "network_tracer__tcp_failure"
	mapTTL              = 10 * time.Second.Nanoseconds()
)

var failureTelemetry = struct {
	failedConnOrphans telemetry.Counter
	failedConnMatches telemetry.Counter
}{
	telemetry.NewCounter(telemetryModuleName, "orphans", []string{}, "Counter measuring the number of orphans after associating failed connections with a closed connection"),
	telemetry.NewCounter(telemetryModuleName, "matches", []string{"type"}, "Counter measuring the number of successful matches of failed connections with closed connections"),
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
	mapCleaner    *ddebpf.MapCleaner[uint64, int64]
	sync.RWMutex
}

// NewFailedConns returns a new FailedConns struct
func NewFailedConns(m *manager.Manager) *FailedConns {
	fc := &FailedConns{
		FailedConnMap: make(map[ebpf.ConnTuple]*FailedConnStats),
	}
	fc.setupMapCleaner(m)
	return fc
}

// upsertConn adds or updates the failed connection in the failed connection map
func (fc *FailedConns) upsertConn(failedConn *ebpf.FailedConn) {
	if _, exists := allowListErrs[failedConn.Reason]; !exists {
		return
	}
	connTuple := failedConn.Tup

	fc.Lock()
	defer fc.Unlock()

	stats, ok := fc.FailedConnMap[connTuple]
	if !ok {
		stats = &FailedConnStats{
			CountByErrCode: make(map[uint32]uint32),
		}
		fc.FailedConnMap[connTuple] = stats
	}

	stats.CountByErrCode[failedConn.Reason]++
	stats.Expiry = time.Now().Add(2 * time.Minute).Unix()
}

// MatchFailedConn increments the failed connection counters for a given connection based on the failed connection map
func (fc *FailedConns) MatchFailedConn(conn *network.ConnectionStats) {
	if fc == nil {
		return
	}
	if conn.Type != network.TCP {
		return
	}
	connTuple := connStatsToTuple(conn)

	fc.RLock()
	defer fc.RUnlock()

	if failedConn, ok := fc.FailedConnMap[connTuple]; ok {
		// found matching failed connection
		conn.TCPFailures = make(map[uint32]uint32)

		for errCode, count := range failedConn.CountByErrCode {
			failureTelemetry.failedConnMatches.Add(1, strconv.Itoa(int(errCode)))
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
	if !c.Source.IsZero() {
		tup.Saddr_l, tup.Saddr_h = util.ToLowHigh(c.Source)
	}
	if !c.Dest.IsZero() {
		tup.Daddr_l, tup.Daddr_h = util.ToLowHigh(c.Dest)
	}
	return tup
}

func (fc *FailedConns) setupMapCleaner(m *manager.Manager) {
	connCloseFlushMap, _, err := m.GetMap(probes.ConnCloseFlushed)
	if err != nil {
		log.Errorf("error getting %v map: %s", probes.ConnCloseFlushed, err)
		return
	}
	mapCleaner, err := ddebpf.NewMapCleaner[uint64, int64](connCloseFlushMap, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	mapCleaner.Clean(time.Second*30, nil, nil, func(now int64, _key uint64, val int64) bool {
		return val > 0 && now-val > mapTTL
	})

	fc.mapCleaner = mapCleaner
}
