// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestProcessCheckRealtimeBeforeStandard(t *testing.T) {
	processCheck, _ := processCheckWithMockProbe(t)

	// If the standard process check hasn't run yet, nothing is returned
	expected := CombinedRunResult{}

	actual, err := processCheck.runRealtime(0)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestProcessCheckRealtimeFirstRun(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	proc3 := makeProcess(3, "foo --version")
	proc4 := makeProcess(4, "foo -bar -bim")
	proc5 := makeProcess(5, "datadog-process-agent --cfgpath datadog.conf")
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(processesByPid, nil)
	probe.On("StatsForPIDs", mock.Anything, mock.Anything).Return(statsByPid, nil)

	// Run the standard process check once to populate last seen pids
	processCheck.run(0, false)

	// The first realtime check returns nothing
	expected := CombinedRunResult{}

	actual, err := processCheck.runRealtime(0)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestProcessCheckRealtimeSecondRun(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	proc3 := makeProcess(3, "foo --version")
	proc4 := makeProcess(4, "foo -bar -bim")
	proc5 := makeProcess(5, "datadog-process-agent --cfgpath datadog.conf")
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(processesByPid, nil)
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
