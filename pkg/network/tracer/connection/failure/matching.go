// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package failure

import (
	"fmt"
	"sync"
	"syscall"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	telemetryModuleName     = "network_tracer__tcp_failure"
	connClosedFlushMapTTL   = 10 * time.Millisecond.Nanoseconds()
	tcpOngoingConnectMapTTL = 30 * time.Minute.Nanoseconds()
)

var failureTelemetry = struct {
	failedConnMatches  telemetry.Counter
	failedConnOrphans  telemetry.Counter
	failedConnsDropped telemetry.Counter
}{
	telemetry.NewCounter(telemetryModuleName, "matches", []string{"type"}, "Counter measuring the number of successful matches of failed connections with closed connections"),
	telemetry.NewCounter(telemetryModuleName, "orphans", []string{}, "Counter measuring the number of orphans after associating failed connections with a closed connection"),
	telemetry.NewCounter(telemetryModuleName, "dropped", []string{}, "Counter measuring the number of dropped failed connections"),
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
	FailedConnMap           map[ebpf.ConnTuple]*FailedConnStats
	maxFailuresBuffered     uint32
	failureTuple            *ebpf.ConnTuple
	connCloseFlushedCleaner *ddebpf.MapCleaner[ebpf.ConnTuple, int64]
	ongoingConnectCleaner   *ddebpf.MapCleaner[ebpf.SkpConn, ebpf.PidTs]
	sync.Mutex
}

// NewFailedConns returns a new FailedConns struct
func NewFailedConns(m *manager.Manager, maxFailedConnsBuffered uint32) *FailedConns {
	fc := &FailedConns{
		FailedConnMap:       make(map[ebpf.ConnTuple]*FailedConnStats),
		maxFailuresBuffered: maxFailedConnsBuffered,
		failureTuple:        &ebpf.ConnTuple{},
	}
	fc.setupMapCleaner(m)
	return fc
}

// upsertConn adds or updates the failed connection in the failed connection map
func (fc *FailedConns) upsertConn(failedConn *ebpf.FailedConn) {
	if fc == nil {
		return
	}

	fc.Lock()
	defer fc.Unlock()

	if len(fc.FailedConnMap) >= int(fc.maxFailuresBuffered) {
		failureTelemetry.failedConnsDropped.Inc()
		return
	}
	connTuple := failedConn.Tup

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
	if fc == nil || conn.Type != network.TCP {
		return
	}

	fc.Lock()
	defer fc.Unlock()

	util.ConnStatsToTuple(conn, fc.failureTuple)

	if failedConn, ok := fc.FailedConnMap[*fc.failureTuple]; ok {
		// found matching failed connection
		conn.TCPFailures = failedConn.CountByErrCode

		for errCode := range failedConn.CountByErrCode {
			failureTelemetry.failedConnMatches.Add(1, unix.ErrnoName(syscall.Errno(errCode)))
		}
		delete(fc.FailedConnMap, *fc.failureTuple)
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

func (fc *FailedConns) setupMapCleaner(m *manager.Manager) {
	connCloseFlushMap, _, err := m.GetMap(probes.ConnCloseFlushed)
	if err != nil {
		log.Errorf("error getting %v map: %s", probes.ConnCloseFlushed, err)
		return
	}
	connFlushedCleaner, err := ddebpf.NewMapCleaner[ebpf.ConnTuple, int64](connCloseFlushMap, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	connFlushedCleaner.Clean(time.Second*1, nil, nil, func(now int64, _ ebpf.ConnTuple, val int64) bool {
		return val > 0 && now-val > connClosedFlushMapTTL
	})

	tcpOngoingConnectPidMap, _, err := m.GetMap(probes.TCPOngoingConnectPid)
	if err != nil {
		log.Errorf("error getting %v map: %s", probes.TCPOngoingConnectPid, err)
		return
	}

	tcpOngoingConnectPidCleaner, err := ddebpf.NewMapCleaner[ebpf.SkpConn, ebpf.PidTs](tcpOngoingConnectPidMap, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	tcpOngoingConnectPidCleaner.Clean(time.Minute*30, nil, nil, func(now int64, _ ebpf.SkpConn, val ebpf.PidTs) bool {
		ts := int64(val.Timestamp)
		return ts > 0 && now-ts > tcpOngoingConnectMapTTL
	})

	fc.connCloseFlushedCleaner = connFlushedCleaner
	fc.ongoingConnectCleaner = tcpOngoingConnectPidCleaner
}
