// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package processlanguage implements the process language collector for
// Workloadmeta.
package process

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type collectorTest struct {
	collector *collector
	probe     *mocks.Probe
	mockClock *clock.Mock
	mockStore workloadmetamock.Mock
}

func setUpCollectorTest(t *testing.T, configOverrides map[string]interface{}) collectorTest {
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: configOverrides}),
		workloadmetafxmock.MockModule(workloadmeta.Params{
			AgentType: workloadmeta.NodeAgent,
		}),
	))

	mockClock := clock.NewMock()
	mockProbe := mocks.NewProbe(t)
	processCollector := newProcessCollector(collectorID, workloadmeta.NodeAgent, mockClock, mockProbe)

	return collectorTest{&processCollector, mockProbe, mockClock, mockStore}
}

func createTestProcess1(pid int32, createTime int64) (*procutil.Process, *workloadmeta.Process) {
	proc := &procutil.Process{
		Pid:     pid,
		Ppid:    6,
		NsPid:   2,
		Name:    "some name",
		Cwd:     "some_directory/path",
		Exe:     "test",
		Comm:    "",
		Cmdline: []string{"c1", "c2", "c3"},
		Uids:    []int32{1, 2, 3, 4},
		Gids:    []int32{1, 2, 3, 4, 5},
		Stats: &procutil.Stats{
			CreateTime:  createTime,
			Status:      "",
			Nice:        0,
			OpenFdCount: 0,
			NumThreads:  0,
			CPUPercent:  nil,
			CPUTime:     nil,
			MemInfo:     nil,
			MemInfoEx:   nil,
			IOStat:      nil,
			IORateStat:  nil,
			CtxSwitches: nil,
		},
	}

	expectedProc := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(proc.Pid)),
		},
		Pid:          proc.Pid,
		Ppid:         proc.Ppid,
		NsPid:        proc.NsPid,
		Name:         proc.Name,
		Cwd:          proc.Cwd,
		Exe:          proc.Exe,
		Comm:         proc.Comm,
		Cmdline:      proc.Cmdline,
		Uids:         proc.Uids,
		Gids:         proc.Gids,
		CreationTime: time.UnixMilli(proc.Stats.CreateTime).UTC(),
	}
	return proc, expectedProc
}

func createTestProcess2(pid int32, createTime int64) (*procutil.Process, *workloadmeta.Process) {
	proc := &procutil.Process{
		Pid:     pid,
		Ppid:    9,
		NsPid:   3,
		Name:    "some name 2",
		Cwd:     "some_directory/path/path2",
		Exe:     "exe",
		Comm:    "hello",
		Cmdline: []string{"c1", "c2", "c3", "c4", "c5"},
		Uids:    []int32{1},
		Gids:    []int32{1, 2},
		Stats: &procutil.Stats{
			CreateTime:  createTime,
			Status:      "",
			Nice:        0,
			OpenFdCount: 0,
			NumThreads:  0,
			CPUPercent:  nil,
			CPUTime:     nil,
			MemInfo:     nil,
			MemInfoEx:   nil,
			IOStat:      nil,
			IORateStat:  nil,
			CtxSwitches: nil,
		},
	}

	expectedProc := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(proc.Pid)),
		},
		Pid:          proc.Pid,
		Ppid:         proc.Ppid,
		NsPid:        proc.NsPid,
		Name:         proc.Name,
		Cwd:          proc.Cwd,
		Exe:          proc.Exe,
		Comm:         proc.Comm,
		Cmdline:      proc.Cmdline,
		Uids:         proc.Uids,
		Gids:         proc.Gids,
		CreationTime: time.UnixMilli(proc.Stats.CreateTime).UTC(),
	}
	return proc, expectedProc
}

// TestCreatedProcessesCollection tests the collector capturing new processes
func TestCreatedProcessesCollection(t *testing.T) {
	collectionInterval := time.Second * 10

	creationTime1 := time.Now().Unix()
	pid1 := int32(1234)
	proc1, expectedProc1 := createTestProcess1(pid1, creationTime1)

	creationTime2 := time.Now().Add(time.Second).Unix()
	pid2 := int32(9999)
	proc2, expectedProc2 := createTestProcess2(pid2, creationTime2)

	for _, tc := range []struct {
		description        string
		configOverrides    map[string]interface{}
		processesToCollect map[int32]*procutil.Process
		expectedProcesses  map[int32]*workloadmeta.Process
	}{
		{
			description:     "single new process",
			configOverrides: map[string]interface{}{},
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				expectedProc1.Pid: expectedProc1,
			},
		},
		{
			description:     "multiple new processes",
			configOverrides: map[string]interface{}{},
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				expectedProc1.Pid: expectedProc1,
				expectedProc2.Pid: expectedProc2,
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			c := setUpCollectorTest(t, tc.configOverrides)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of 3 lines below when configuration is sorted as Start() is currently
			// by default disabled
			c.collector.store = c.mockStore
			go c.collector.collect(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Times(1)
			// update clock to trigger processing
			c.mockClock.Add(collectionInterval)

			assert.EventuallyWithT(t, func(cT *assert.CollectT) {
				for pid, expectedProc := range tc.expectedProcesses {
					actualProc, err := c.mockStore.GetProcess(pid)
					assert.NoError(cT, err)
					assert.Equal(cT, expectedProc, actualProc)
				}
			}, time.Second, time.Millisecond*100)
		})
	}
}

// TestCreatedProcessesCollection tests the collector capturing lifecycle of a process (creation, deletion)
func TestProcessLifecycleCollection(t *testing.T) {
	collectionInterval := time.Second * 10
	creationTime1 := time.Now().Unix()
	pid1 := int32(1234)
	proc1, expectedProc1 := createTestProcess1(pid1, creationTime1)

	creationTime2 := time.Now().Add(time.Second).Unix()
	pid2 := int32(9999)
	proc2, expectedProc2 := createTestProcess2(pid2, creationTime2)

	// same pid as proc1 but different creation time
	proc3, expectedProc3 := createTestProcess1(pid1, creationTime2)

	// same pid as proc2 but different creation time
	creationTime3 := time.Now().Add(2 * time.Second).Unix()
	proc4, expectedProc4 := createTestProcess1(pid2, creationTime3)

	for _, tc := range []struct {
		description              string
		configOverrides          map[string]interface{}
		processesToCollectA      map[int32]*procutil.Process
		processesToCollectB      map[int32]*procutil.Process
		expectedDeletedProcesses []*workloadmeta.Process
		expectedLiveProcesses    []*workloadmeta.Process
	}{
		{
			description:     "2 new processes and 1 finishes",
			configOverrides: map[string]interface{}{},
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			processesToCollectB: map[int32]*procutil.Process{
				proc2.Pid: proc2,
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				expectedProc1,
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				expectedProc2,
			},
		},
		{
			description:     "2 new processes and 2 finishes",
			configOverrides: map[string]interface{}{},
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			processesToCollectB:   map[int32]*procutil.Process{},
			expectedLiveProcesses: []*workloadmeta.Process{},
			expectedDeletedProcesses: []*workloadmeta.Process{
				expectedProc1,
				expectedProc2,
			},
		},
		{
			description:     "2 new processes, 2 finish, but 2 new processes with the same pid",
			configOverrides: map[string]interface{}{},
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			processesToCollectB: map[int32]*procutil.Process{
				proc3.Pid: proc3,
				proc4.Pid: proc4,
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				expectedProc3, expectedProc4,
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				expectedProc1, expectedProc2,
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			c := setUpCollectorTest(t, tc.configOverrides)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of 3 lines below when configuration is sorted as Start() is currently
			// by default disabled
			c.collector.store = c.mockStore
			go c.collector.collect(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollectA, nil).Times(1)
			// update clock to trigger processing
			c.mockClock.Add(collectionInterval)
			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollectB, nil).Times(1)
			// update clock to trigger processing
			c.mockClock.Add(collectionInterval)

			assert.EventuallyWithT(t, func(cT *assert.CollectT) {
				actualProcs := c.mockStore.ListProcesses()
				mapActualProcs := make(map[int32]*workloadmeta.Process, len(actualProcs))
				for _, proc := range actualProcs {
					mapActualProcs[proc.Pid] = proc
				}

				for _, expectedLiveProc := range tc.expectedLiveProcesses {
					actualProc, exists := mapActualProcs[expectedLiveProc.Pid]
					assert.True(cT, exists)
					assert.Equal(cT, expectedLiveProc, actualProc)
				}

				for _, expectedDeletedProc := range tc.expectedDeletedProcesses {
					actualProc, exists := mapActualProcs[expectedDeletedProc.Pid]

					// the same process pid can exist so we ensure it is a different process by checking the creation time
					if exists {
						assert.NotEqual(cT, expectedDeletedProc.CreationTime, actualProc.CreationTime)
					}

				}
			}, time.Second, time.Millisecond*100)
		})
	}
}
