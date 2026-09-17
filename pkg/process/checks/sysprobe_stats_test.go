// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"math/rand"
	"testing"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func makeProcStatsWithPerm(pid int32) *process.ProcStatsWithPerm {
	r := rand.New(rand.NewSource(int64(pid)))
	return &process.ProcStatsWithPerm{
		OpenFDCount: r.Int31(),
		ReadCount:   r.Int63(),
		WriteCount:  r.Int63(),
		ReadBytes:   r.Int63(),
	}
}

func TestMergeProcWithSysprobeStats(t *testing.T) {
	assertMatchesSysProbeStats := func(t *testing.T, proc *procutil.Process, stats *process.ProcStatsWithPerm) {
		assert.Equal(t, stats.OpenFDCount, proc.Stats.OpenFdCount)
		assert.Equal(t, stats.ReadCount, proc.Stats.IOStat.ReadCount)
		assert.Equal(t, stats.WriteCount, proc.Stats.IOStat.WriteCount)
		assert.Equal(t, stats.ReadBytes, proc.Stats.IOStat.ReadBytes)
		assert.Equal(t, stats.WriteBytes, proc.Stats.IOStat.WriteBytes)
	}
	hasSysProbeStats := func(proc *procutil.Process) bool {
		return !proc.Stats.IOStat.IsZeroValue() && proc.Stats.OpenFdCount != 0
	}

	t.Run("pids match", func(t *testing.T) {
		proc1 := makeProcess(1, "git clone google.com")
		proc2 := makeProcess(2, "mine-bitcoins -all -x")

		proc1Stats := makeProcStatsWithPerm(1)
		proc2Stats := makeProcStatsWithPerm(2)

		mergeProcWithSysprobeStats(map[int32]*procutil.Process{1: proc1, 2: proc2}, &process.ProcStatsWithPermByPID{
			StatsByPID: map[int32]*process.ProcStatsWithPerm{1: proc1Stats, 2: proc2Stats},
		})

		assertMatchesSysProbeStats(t, proc1, proc1Stats)
		assertMatchesSysProbeStats(t, proc2, proc2Stats)
	})

	t.Run("missing pid", func(t *testing.T) {
		proc1 := makeProcess(1, "git clone google.com")
		proc2 := makeProcess(2, "mine-bitcoins -all -x")

		proc1Stats := makeProcStatsWithPerm(1)

		mergeProcWithSysprobeStats(map[int32]*procutil.Process{1: proc1, 2: proc2}, &process.ProcStatsWithPermByPID{
			StatsByPID: map[int32]*process.ProcStatsWithPerm{1: proc1Stats},
		})

		assertMatchesSysProbeStats(t, proc1, proc1Stats)
		assert.False(t, hasSysProbeStats(proc2))
	})

	t.Run("missing proc", func(t *testing.T) {
		proc2 := makeProcess(2, "mine-bitcoins -all -x")

		proc1Stats := makeProcStatsWithPerm(1)
		proc2Stats := makeProcStatsWithPerm(2)

		mergeProcWithSysprobeStats(map[int32]*procutil.Process{2: proc2}, &process.ProcStatsWithPermByPID{
			StatsByPID: map[int32]*process.ProcStatsWithPerm{1: proc1Stats, 2: proc2Stats},
		})

		assertMatchesSysProbeStats(t, proc2, proc2Stats)
	})

	t.Run("zombie stats remain empty", func(t *testing.T) {
		proc := makeProcess(1, "zombie")
		proc.Stats.Status = "Z"
		proc.Stats.OpenFdCount = 0
		proc.Stats.IOStat = &procutil.IOCountersStat{}

		mergeProcWithSysprobeStats(map[int32]*procutil.Process{1: proc}, &process.ProcStatsWithPermByPID{
			StatsByPID: map[int32]*process.ProcStatsWithPerm{1: makeProcStatsWithPerm(1)},
		})

		assert.Zero(t, proc.Stats.OpenFdCount)
		assert.True(t, proc.Stats.IOStat.IsZeroValue())
	})
}

func TestPIDsForSystemProbeStats(t *testing.T) {
	running := makeProcess(1, "running")
	zombie := makeProcess(2, "zombie")
	zombie.Stats.Status = "Z"

	assert.ElementsMatch(t, []int32{1}, pidsForSystemProbeStats(map[int32]*procutil.Process{
		1: running,
		2: zombie,
	}))
}

func TestPIDsForRealtimeSystemProbeStats(t *testing.T) {
	pids := []int32{1, 2, 3}
	stats := map[int32]*procutil.Stats{
		1: {Status: "R"},
		2: {Status: "Z"},
	}

	assert.Equal(t, []int32{1}, pidsForRealtimeSystemProbeStats(pids, stats))
}
