// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/jvmmemory/model"
)

func poolKey(tgid uint32, name string) ebpfPoolKey {
	key := ebpfPoolKey{Tgid: tgid}
	copy(key.Name[:], name)
	return key
}

func TestKtimeToWall(t *testing.T) {
	c := clock{monotonicNs: 5_000_000_000, wallNs: 1_700_000_000_000_000_000}
	assert.Equal(t, int64(1_700_000_000_000_000_000), c.ktimeToWall(5_000_000_000))
	assert.Equal(t, int64(1_699_999_998_000_000_000), c.ktimeToWall(3_000_000_000))
	assert.Zero(t, c.ktimeToWall(0), "no timestamp")
}

func TestBuildStats(t *testing.T) {
	c := clock{monotonicNs: 10_000, wallNs: 1_000_000}
	pools := map[ebpfPoolKey]ebpfPoolValue{
		poolKey(200, "Metaspace"):     {Init: math.MaxUint64, Used: 10, Committed: 20, Max: math.MaxUint64, Timestamp: 9_000},
		poolKey(100, "G1 Old Gen"):    {Init: 1, Used: 2, Committed: 3, Max: 4, Timestamp: 8_000},
		poolKey(100, "G1 Eden Space"): {Init: 5, Used: 6, Committed: 7, Max: math.MaxUint64, Timestamp: 7_000},
		poolKey(300, "Tenured Gen"):   {Used: 1, Timestamp: 1},
	}
	gcs := map[uint32]ebpfGCStats{
		100: {Pause_start_ns: 9_500, Pauses: 4, Full_pauses: 1, Pause_time_ns: 300, Last_pause_end_ns: 9_900, Pause_full: 1},
		// a JVM which did not report its pools yet
		400: {Pauses: 1, Pause_time_ns: 10, Last_pause_end_ns: 5_000},
		300: {Pauses: 1},
	}
	alive := func(pid uint32) bool { return pid != 300 }
	containerIDs := map[uint32]string{100: "abc"}
	containerID := func(pid uint32) string { return containerIDs[pid] }

	expected := []model.JVMStats{
		{
			PID:         100,
			ContainerID: "abc",
			Pools: []model.PoolStats{
				{Name: "G1 Eden Space", Init: 5, Used: 6, Committed: 7, Max: model.UndefinedSize, LastUpdate: 997_000},
				{Name: "G1 Old Gen", Init: 1, Used: 2, Committed: 3, Max: 4, LastUpdate: 998_000},
			},
			GC: model.GCStats{Pauses: 4, FullPauses: 1, PauseTimeNs: 300, LastPauseEnd: 999_900},
		},
		{
			PID: 200,
			Pools: []model.PoolStats{
				{Name: "Metaspace", Init: model.UndefinedSize, Used: 10, Committed: 20, Max: model.UndefinedSize, LastUpdate: 999_000},
			},
		},
		{
			PID:   400,
			Pools: []model.PoolStats{},
			GC:    model.GCStats{Pauses: 1, PauseTimeNs: 10, LastPauseEnd: 995_000},
		},
	}
	assert.Equal(t, expected, buildStats(pools, gcs, c, alive, containerID))

	assert.Empty(t, buildStats(nil, nil, c, alive, containerID))
}
