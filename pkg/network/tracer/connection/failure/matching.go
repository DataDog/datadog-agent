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
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

var (
	allowListErrs = map[uint32]struct{}{
		ebpf.TCPFailureConnReset:   {}, // Connection reset by peer
		ebpf.TCPFailureConnTimeout: {}, // Connection timed out
		ebpf.TCPFailureConnRefused: {}, // Connection refused
	}

	telemetryModuleName = "network_tracer__tcp_failure"
	mapTTL              = 10 * time.Millisecond.Nanoseconds()

	tuplePool = ddsync.NewDefaultTypedPool[ebpf.ConnTuple]()
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

func (t FailedConnStats) reset() {
	for k := range t.CountByErrCode {
		delete(t.CountByErrCode, k)
	}
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
	FailedConnMap       map[ebpf.ConnTuple]*FailedConnStats
	maxFailuresBuffered uint32
	failureTuple        *ebpf.ConnTuple
	mapCleaner          *ddebpf.MapCleaner[ebpf.ConnTuple, int64]
	pool                *ddsync.TypedPool[FailedConnStats]
	sync.RWMutex
}

// NewFailedConns returns a new FailedConns struct
func NewFailedConns(m *manager.Manager, maxFailedConnsBuffered uint32) *FailedConns {
	fc := &FailedConns{
		FailedConnMap:       make(map[ebpf.ConnTuple]*FailedConnStats),
		maxFailuresBuffered: maxFailedConnsBuffered,
		failureTuple:        &ebpf.ConnTuple{},
		pool: ddsync.NewTypedPool(func() *FailedConnStats {
			return &FailedConnStats{
				CountByErrCode: make(map[uint32]uint32),
			}
		}),
	}
	fc.setupMapCleaner(m)
	return fc
}

// upsertConn adds or updates the failed connection in the failed connection map
func (fc *FailedConns) upsertConn(failedConn *ebpf.FailedConn) {
	if _, exists := allowListErrs[failedConn.Reason]; !exists {
		return
	}
	if len(fc.FailedConnMap) >= int(fc.maxFailuresBuffered) {
		failureTelemetry.failedConnsDropped.Inc()
		return
	}
	connTuple := failedConn.Tup

	fc.Lock()
	defer fc.Unlock()

	stats, ok := fc.FailedConnMap[connTuple]
	if !ok {
		stats = fc.pool.Get()
		stats.reset()
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
	util.ConnStatsToTuple(conn, fc.failureTuple)

	fc.RLock()
	foundMatch := false
	var failedConn *FailedConnStats

	if failedConn, ok := fc.FailedConnMap[*fc.failureTuple]; ok {
		foundMatch = true
		// found matching failed connection
		conn.TCPFailures = make(map[uint32]uint32)

		for errCode, count := range failedConn.CountByErrCode {
			failureTelemetry.failedConnMatches.Add(1, strconv.Itoa(int(errCode)))
			conn.TCPFailures[errCode] += count
		}
	}
	fc.RUnlock()
	if foundMatch {
		fc.Lock()
		delete(fc.FailedConnMap, *fc.failureTuple)
		fc.pool.Put(failedConn)
		fc.Unlock()
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

	mapCleaner.Clean(time.Second*1, nil, nil, func(now int64, _key ebpf.ConnTuple, val int64) bool {
		return val > 0 && now-val > mapTTL
	})

	fc.mapCleaner = mapCleaner
}
