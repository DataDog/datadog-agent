// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	metricsmock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func processCheckWithMockProbe(t *testing.T) (*ProcessCheck, *mocks.Probe) {
	t.Helper()
	probe := mocks.NewProbe(t)
	return &ProcessCheck{
		probe: probe,
		sysInfo: &model.SystemInfo{
			Cpus: []*model.CPUInfo{
				{CoreId: "1"},
				{CoreId: "2"},
				{CoreId: "3"},
				{CoreId: "4"},
			},
		},
		containerProvider: mockContainerProvider(t),
	}, probe
}

// TODO: create a centralized, easy way to mock this
func mockContainerProvider(t *testing.T) util.ContainerProvider {
	t.Helper()

	// Metrics provider
	metricsCollector := metricsmock.NewCollector("foo")
	metricsProvider := metricsmock.NewMetricsProvider()
	metricsProvider.RegisterConcreteCollector(provider.RuntimeNameContainerd, metricsCollector)
	metricsProvider.RegisterConcreteCollector(provider.RuntimeNameGarden, metricsCollector)

	// Workload meta + tagger
	metadataProvider := workloadmeta.NewMockStore()
	fakeTagger := local.NewFakeTagger()
	tagger.SetDefaultTagger(fakeTagger)
	defer tagger.SetDefaultTagger(nil)

	// Finally, container provider
	filter, err := containers.GetPauseContainerFilter()
	assert.NoError(t, err)
	return util.NewContainerProvider(metricsProvider, metadataProvider, filter)
}

func TestProcessCheckFirstRun(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	pids := []int32{}
	for pid := range processesByPid {
		pids = append(pids, pid)
	}

	ProcessNotify.UpdateCreateTimes(map[int32]int64{})
	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	expected := &RunResult{}

	actual, err := processCheck.run(config.NewDefaultAgentConfig(), 0, false)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)

	// Create times are always updated on process collection
	expectedCreateTimes := map[int32]int64{
		1: now,
		2: now + 1,
		3: now + 2,
		4: now + 3,
		5: now + 2,
	}

	assert.Equal(t, expectedCreateTimes, ProcessNotify.GetCreateTimes(pids))
}

func TestProcessCheckSecondRun(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	pids := []int32{}
	for pid := range processesByPid {
		pids = append(pids, pid)
	}

	ProcessNotify.UpdateCreateTimes(map[int32]int64{})
	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(config.NewDefaultAgentConfig(), 0, false)
	require.NoError(t, err)
	assert.Equal(t, &RunResult{}, first)

	expected := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc2)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc3)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc4)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc5)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
	}
	actual, err := processCheck.run(config.NewDefaultAgentConfig(), 0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, actual.Standard) // ordering is not guaranteed
	assert.Nil(t, actual.RealTime)

	// Create times are always updated on process collection
	expectedCreateTimes := map[int32]int64{
		1: now,
		2: now + 1,
		3: now + 2,
		4: now + 3,
		5: now + 2,
	}

	assert.Equal(t, expectedCreateTimes, ProcessNotify.GetCreateTimes(pids))
}

func TestProcessCheckWithRealtime(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	proc3 := makeProcess(3, "foo --version")
	proc4 := makeProcess(4, "foo -bar -bim")
	proc5 := makeProcess(5, "datadog-process-agent --cfgpath datadog.conf")
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(config.NewDefaultAgentConfig(), 0, true)
	require.NoError(t, err)
	assert.Equal(t, &RunResult{}, first)

	expectedProcs := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc2)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc3)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc4)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc5)},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.sysInfo,
		},
	}

	expectedStats := makeProcessStatModels(t, proc1, proc2, proc3, proc4, proc5)
	actual, err := processCheck.run(config.NewDefaultAgentConfig(), 0, true)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedProcs, actual.Standard) // ordering is not guaranteed
	require.Len(t, actual.RealTime, 1)
	rt := actual.RealTime[0].(*model.CollectorRealTime)
	assert.ElementsMatch(t, expectedStats, rt.Stats)
	assert.Equal(t, int32(1), rt.GroupSize)
	assert.Equal(t, int32(len(processCheck.sysInfo.Cpus)), rt.NumCpus)
}
