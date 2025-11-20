// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"math"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestProcessCheckRealtimeBeforeStandard(t *testing.T) {
	processCheck, _, _ := processCheckWithMocks(t)

	// If the standard process check hasn't run yet, nothing is returned
	expected := CombinedRunResult{}

	actual, err := processCheck.runRealtime(0)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestProcessCheckRealtimeFirstRun(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	proc3 := makeProcess(3, "foo --version")
	proc4 := makeProcess(4, "foo -bar -bim")
	proc5 := makeProcess(5, "datadog-process-agent --cfgpath datadog.conf")
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	probe.On("StatsForPIDs", mock.Anything, mock.Anything).Return(statsByPid, nil)
	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

	// Run the standard process check once to populate last seen pids
	processCheck.run(0, false)

	// The first realtime check returns nothing
	expected := CombinedRunResult{}

	actual, err := processCheck.runRealtime(0)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestProcessCheckRealtimeSecondRun(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	proc3 := makeProcess(3, "foo --version")
	proc4 := makeProcess(4, "foo -bar -bim")
	proc5 := makeProcess(5, "datadog-process-agent --cfgpath datadog.conf")
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)
	probe.On("StatsForPIDs", mock.Anything, mock.Anything).Return(statsByPid, nil)

	// Run the standard process check once to populate last seen pids
	processCheck.run(0, false)

	// The first realtime check returns nothing
	first, err := processCheck.runRealtime(0)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	expected := makeProcessStatModels(t, proc1, proc2, proc3, proc4, proc5)
	actual, err := processCheck.runRealtime(0)
	require.NoError(t, err)
	require.Len(t, actual.RealtimePayloads(), 1)
	rt := actual.RealtimePayloads()[0].(*model.CollectorRealTime)
	assert.ElementsMatch(t, expected, rt.Stats)
	assert.Equal(t, int32(1), rt.GroupSize)
	assert.Equal(t, int32(len(processCheck.hostInfo.SystemInfo.Cpus)), rt.NumCpus)
}

// TestFmtProcessStats test the chunking logic of fmtProcessStats
func TestFmtProcessStats(t *testing.T) {
	procs := map[int32]*procutil.Stats{
		1: makeProcessStats(),
		2: makeProcessStats(),
		3: makeProcessStats(),
	}
	lastProcs := map[int32]*procutil.Stats{
		1: makeProcessStats(),
		2: makeProcessStats(),
		3: makeProcessStats(),
	}

	type testCase struct {
		description        string
		maxBatchSize       int
		expectedNumChunks  int
		expectedChunkSizes []int
	}
	tests := []testCase{
		{
			description:        "Chunking - max batch size 1",
			maxBatchSize:       1,
			expectedNumChunks:  3,
			expectedChunkSizes: []int{1, 1, 1},
		},
		{
			description:        "Chunking - max batch size 2",
			maxBatchSize:       2,
			expectedNumChunks:  2,
			expectedChunkSizes: []int{2, 1},
		},
		{
			description:        "No chunking - max batch size",
			maxBatchSize:       math.MaxInt,
			expectedNumChunks:  1,
			expectedChunkSizes: []int{3},
		},
		{
			description:        "No chunking - max batch size 0",
			maxBatchSize:       0,
			expectedNumChunks:  1,
			expectedChunkSizes: []int{3},
		},
		{
			description:        "No chunking - max batch size 10",
			maxBatchSize:       10,
			expectedNumChunks:  1,
			expectedChunkSizes: []int{3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			chunked := fmtProcessStats(tc.maxBatchSize, procs, lastProcs, map[int]string{}, cpu.TimesStat{}, cpu.TimesStat{}, time.Now().Add(-time.Second), time.Now())
			assert.Len(t, chunked, tc.expectedNumChunks)
			for i, size := range tc.expectedChunkSizes {
				assert.Len(t, chunked[i], size)
			}
		})
	}
}
