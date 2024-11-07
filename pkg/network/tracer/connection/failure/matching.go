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
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	telemetryModuleName   = "network_tracer__tcp_failure"
	connClosedFlushMapTTL = 10 * time.Millisecond.Nanoseconds()
)

var failureTelemetry = struct {
	failedConnMatches        telemetry.Counter
	failedConnOrphans        telemetry.Counter
	failedConnsDropped       telemetry.Counter
	closedConnFlushedCleaned telemetry.Counter
}{
	telemetry.NewCounter(telemetryModuleName, "matches", []string{"type"}, "Counter measuring the number of successful matches of failed connections with closed connections"),
	telemetry.NewCounter(telemetryModuleName, "orphans", []string{}, "Counter measuring the number of orphans after associating failed connections with a closed connection"),
	telemetry.NewCounter(telemetryModuleName, "dropped", []string{}, "Counter measuring the number of dropped failed connections"),
	telemetry.NewCounter(telemetryModuleName, "closed_conn_flushed_cleaned", []string{}, "Counter measuring the number of conn_close_flushed entries cleaned in userspace"),
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

// FailedConns is a struct to hold failed connections
type FailedConns struct {
	FailedConnMap           map[network.ConnectionTuple]*FailedConnStats
	maxFailuresBuffered     uint32
	connCloseFlushedCleaner *ddebpf.MapCleaner[ebpf.ConnTuple, int64]
	sync.Mutex
}

// NewFailedConns returns a new FailedConns struct
func NewFailedConns(m *manager.Manager, maxFailedConnsBuffered uint32) *FailedConns {
	fc := &FailedConns{
		FailedConnMap:       make(map[network.ConnectionTuple]*FailedConnStats),
		maxFailuresBuffered: maxFailedConnsBuffered,
	}
	fc.setupMapCleaner(m)
	return fc
}

// UpsertConn adds or updates the failed connection in the failed connection map
func (fc *FailedConns) UpsertConn(failedConn *Conn) {
	if fc == nil {
		return
	}

	fc.Lock()
	defer fc.Unlock()

	if len(fc.FailedConnMap) >= int(fc.maxFailuresBuffered) {
		failureTelemetry.failedConnsDropped.Inc()
		return
	}
	connTuple := failedConn.Tuple()

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

	if failedConn, ok := fc.FailedConnMap[conn.ConnectionTuple]; ok {
		// found matching failed connection
		conn.TCPFailures = failedConn.CountByErrCode

		for errCode := range failedConn.CountByErrCode {
			failureTelemetry.failedConnMatches.Add(1, unix.ErrnoName(syscall.Errno(errCode)))
		}
		delete(fc.FailedConnMap, conn.ConnectionTuple)
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
	mapCleaner, err := ddebpf.NewMapCleaner[ebpf.ConnTuple, int64](connCloseFlushMap, 1024)
	if err != nil {
		log.Errorf("error creating map cleaner: %s", err)
		return
	}

	mapCleaner.Clean(time.Second*1, nil, nil, func(now int64, _ ebpf.ConnTuple, val int64) bool {
		expired := val > 0 && now-val > connClosedFlushMapTTL
		if expired {
			failureTelemetry.closedConnFlushedCleaned.Inc()
		}
		return expired
	})

	fc.connCloseFlushedCleaner = mapCleaner
}
