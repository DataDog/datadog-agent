// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

func makeContainer(id string) *model.Container {
	return &model.Container{
		Id: id,
	}
}

//nolint:deadcode,unused
func procsToHash(procs []*procutil.Process) (procsByPid map[int32]*procutil.Process) {
	procsByPid = make(map[int32]*procutil.Process)
	for _, p := range procs {
		procsByPid[p.Pid] = p
	}
	return
}

func makeProcess(pid int32, cmdline string) *procutil.Process {
	return &procutil.Process{
		Pid:     pid,
		Cmdline: strings.Split(cmdline, " "),
		Stats: &procutil.Stats{
			CPUTime:     &procutil.CPUTimesStat{},
			MemInfo:     &procutil.MemoryInfoStat{},
			MemInfoEx:   &procutil.MemoryInfoExStat{},
			IOStat:      &procutil.IOCountersStat{},
			CtxSwitches: &procutil.NumCtxSwitchesStat{},
		},
	}
}

//nolint:deadcode,unused
// procMsgsVerification takes raw containers and processes and make sure the chunked messages have all data, and each chunk has the correct grouping
func procMsgsVerification(t *testing.T, msgs []model.MessageBody, rawContainers []*containers.Container, rawProcesses []*procutil.Process, maxSize int, cfg *config.AgentConfig) {
	actualProcs := 0
	for _, msg := range msgs {
		payload := msg.(*model.CollectorProc)

		if len(payload.Containers) > 0 {
			// assume no blacklist involved
			assert.Equal(t, len(rawContainers), len(payload.Containers))

			procsByPid := make(map[int32]struct{}, len(payload.Processes))
			for _, p := range payload.Processes {
				procsByPid[p.Pid] = struct{}{}
			}

			// make sure all containerized processes are in the payload
			containeredProcs := 0
			for _, ctr := range rawContainers {
				for _, pid := range ctr.Pids {
					assert.Contains(t, procsByPid, pid)
					containeredProcs++
				}
			}
			assert.Equal(t, len(payload.Processes), containeredProcs)

			actualProcs += containeredProcs
		} else {
			assert.True(t, len(payload.Processes) <= maxSize)
			actualProcs += len(payload.Processes)
		}
		assert.Equal(t, cfg.ContainerHostType, payload.ContainerHostType)
	}
	assert.Equal(t, len(rawProcesses), actualProcs)
}

func TestPercentCalculation(t *testing.T) {
	// Capping at NUM CPU * 100 if we get odd values for delta-{Proc,Time}
	assert.True(t, floatEquals(calculatePct(100, 50, 1), 100))

	// Zero deltaTime case
	assert.True(t, floatEquals(calculatePct(100, 0, 8), 0.0))

	assert.True(t, floatEquals(calculatePct(0, 8.08, 8), 0.0))
	if runtime.GOOS != "windows" {
		assert.True(t, floatEquals(calculatePct(100, 200, 2), 100))
		assert.True(t, floatEquals(calculatePct(0.04, 8.08, 8), 3.960396))
		assert.True(t, floatEquals(calculatePct(1.09, 8.08, 8), 107.920792))
	}
}

func TestRateCalculation(t *testing.T) {
	now := time.Now()
	prev := now.Add(-1 * time.Second)
	var empty time.Time
	assert.True(t, floatEquals(calculateRate(5, 1, prev), 4))
	assert.True(t, floatEquals(calculateRate(5, 1, prev.Add(-2*time.Second)), float32(1.33333333)))
	assert.True(t, floatEquals(calculateRate(5, 1, now), 0))
	assert.True(t, floatEquals(calculateRate(5, 0, prev), 0))
	assert.True(t, floatEquals(calculateRate(5, 1, empty), 0))

	// Underflow on cur - prev
	assert.True(t, floatEquals(calculateRate(0, 1, prev), 0))
}

func TestFormatIO(t *testing.T) {
	fp := &procutil.Stats{
		IOStat: &procutil.IOCountersStat{
			ReadCount:  6,
			WriteCount: 8,
			ReadBytes:  10,
			WriteBytes: 12,
		},
	}

	last := &procutil.IOCountersStat{
		ReadCount:  1,
		WriteCount: 2,
		ReadBytes:  3,
		WriteBytes: 4,
	}

	// fp.IOStat is nil
	assert.NotNil(t, formatIO(&procutil.Stats{}, last, time.Now().Add(-2*time.Second)))

	// IOStats have 0 values
	result := formatIO(&procutil.Stats{IOStat: &procutil.IOCountersStat{}}, last, time.Now().Add(-2*time.Second))
	assert.Equal(t, float32(0), result.ReadRate)
	assert.Equal(t, float32(0), result.WriteRate)
	assert.Equal(t, float32(0), result.ReadBytesRate)
	assert.Equal(t, float32(0), result.WriteBytesRate)

	// Elapsed time < 1s
	assert.NotNil(t, formatIO(fp, last, time.Now()))

	// IOStats have permission problem
	result = formatIO(&procutil.Stats{IOStat: &procutil.IOCountersStat{
		ReadCount:  -1,
		WriteCount: -1,
		ReadBytes:  -1,
		WriteBytes: -1,
	}}, last, time.Now().Add(-1*time.Second))
	assert.Equal(t, float32(-1), result.ReadRate)
	assert.Equal(t, float32(-1), result.WriteRate)
	assert.Equal(t, float32(-1), result.ReadBytesRate)
	assert.Equal(t, float32(-1), result.WriteBytesRate)

	result = formatIO(fp, last, time.Now().Add(-1*time.Second))
	require.NotNil(t, result)
	assert.Equal(t, float32(5), result.ReadRate)
	assert.Equal(t, float32(6), result.WriteRate)
	assert.Equal(t, float32(7), result.ReadBytesRate)
	assert.Equal(t, float32(8), result.WriteBytesRate)
}

func TestFormatNetworks(t *testing.T) {
	for _, tc := range []struct {
		connsByPID map[int32][]*model.Connection
		interval   int
		pid        int32
		expected   *model.ProcessNetworks
	}{
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 2,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 5, BytesRate: 150},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 10,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 1, BytesRate: 30},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 20,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 0.5, BytesRate: 15},
		},
		{
			connsByPID: nil,
			interval:   20,
			pid:        1,
			expected:   &model.ProcessNetworks{ConnectionRate: 0, BytesRate: 0},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 10,
			pid:      2,
			expected: &model.ProcessNetworks{ConnectionRate: 0, BytesRate: 0},
		},
	} {
		result := formatNetworks(tc.connsByPID[tc.pid], tc.interval)
		assert.EqualValues(t, tc.expected, result)
	}
}

func floatEquals(a, b float32) bool {
	var e float32 = 0.00000001 // Difference less than some epsilon
	return a-b < e && b-a < e
}

func yieldConnections(count int) []*model.Connection {
	result := make([]*model.Connection, count)
	for i := 0; i < count; i++ {
		result[i] = &model.Connection{LastBytesReceived: 10, LastBytesSent: 20}
	}
	return result
}
