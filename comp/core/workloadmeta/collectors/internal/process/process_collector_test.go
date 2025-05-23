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
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
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
	processEventCh := make(chan *Event)
	processCollector := newProcessCollector(collectorID, mockStore, workloadmeta.NodeAgent, mockClock, mockProbe, processEventCh, make(map[int32]*procutil.Process))

	return collectorTest{&processCollector, mockProbe, mockClock, mockStore}
}

// start used for testing purposes while we wait for configuration logic to be sorted out
func (c *collector) start(ctx context.Context, store workloadmeta.Component, collectionInterval time.Duration) error {
	c.store = store
	go c.collect(ctx, c.clock.Ticker(collectionInterval))
	go c.stream(ctx)
	return nil
}

// TestCreatedProcessesCollection tests the collector capturing new processes
func TestCreatedProcessesCollection(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.DefaultAgent)

	creationTime1 := time.Now().Unix()
	creationTime2 := time.Now().Add(time.Second).Unix()
	collectionInterval := time.Second * 10

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
				1234: {
					Pid:     1234,
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
						CreateTime:  creationTime1,
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
				},
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				1234: {
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},

					Pid:          1234,
					Ppid:         6,
					NsPid:        2,
					Name:         "some name",
					Cwd:          "some_directory/path",
					Exe:          "test",
					Comm:         "",
					Cmdline:      []string{"c1", "c2", "c3"},
					Uids:         []int32{1, 2, 3, 4},
					Gids:         []int32{1, 2, 3, 4, 5},
					CreationTime: time.UnixMilli(creationTime1).UTC(),
				},
			},
		},
		{
			description:     "multiple new processes",
			configOverrides: map[string]interface{}{},
			processesToCollect: map[int32]*procutil.Process{
				1234: {
					Pid:     1234,
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
						CreateTime:  creationTime1,
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
				},
				9999: {
					Pid:     9999,
					Ppid:    8,
					NsPid:   3,
					Name:    "some name 9999",
					Cwd:     "some_directory/path/for",
					Exe:     "exe",
					Comm:    "something",
					Cmdline: []string{"c1", "c2", "c3", "c4"},
					Uids:    []int32{},
					Gids:    []int32{},
					Stats: &procutil.Stats{
						CreateTime:  creationTime2,
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
				},
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				1234: {
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},

					Pid:          1234,
					Ppid:         6,
					NsPid:        2,
					Name:         "some name",
					Cwd:          "some_directory/path",
					Exe:          "test",
					Comm:         "",
					Cmdline:      []string{"c1", "c2", "c3"},
					Uids:         []int32{1, 2, 3, 4},
					Gids:         []int32{1, 2, 3, 4, 5},
					CreationTime: time.UnixMilli(creationTime1).UTC(),
				},
				9999: {
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "9999",
					},
					Pid:          9999,
					Ppid:         8,
					NsPid:        3,
					Name:         "some name 9999",
					Cwd:          "some_directory/path/for",
					Exe:          "exe",
					Comm:         "something",
					Cmdline:      []string{"c1", "c2", "c3", "c4"},
					Uids:         []int32{},
					Gids:         []int32{},
					CreationTime: time.UnixMilli(creationTime2).UTC(),
				},
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			c := setUpCollectorTest(t, tc.configOverrides)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of start() once we have the config file logic finished
			err := c.collector.start(ctx, c.mockStore, collectionInterval)
			assert.NoError(t, err)

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
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.DefaultAgent)

	collectionInterval := time.Second * 10
	creationTime := time.Now()
	creationTime1 := creationTime.Unix()
	creationTime2 := creationTime.Add(time.Second).Unix()
	creationTime3 := creationTime.Add(2 * time.Second).Unix()

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
				1234: {
					Pid:     1234,
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
						CreateTime:  creationTime1,
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
				},
				9999: {
					Pid:     9999,
					Ppid:    8,
					NsPid:   3,
					Name:    "some name 9999",
					Cwd:     "some_directory/path/for",
					Exe:     "exe",
					Comm:    "something",
					Cmdline: []string{"c1", "c2", "c3", "c4"},
					Uids:    []int32{},
					Gids:    []int32{},
					Stats: &procutil.Stats{
						CreateTime:  creationTime2,
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
				},
			},
			processesToCollectB: map[int32]*procutil.Process{
				9999: {
					Pid:     9999,
					Ppid:    8,
					NsPid:   3,
					Name:    "some name 9999",
					Cwd:     "some_directory/path/for",
					Exe:     "exe",
					Comm:    "something",
					Cmdline: []string{"c1", "c2", "c3", "c4"},
					Uids:    []int32{},
					Gids:    []int32{},
					Stats: &procutil.Stats{
						CreateTime:  creationTime2,
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
				},
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},
					Pid:          1234,
					Ppid:         6,
					NsPid:        2,
					Name:         "some name",
					Cwd:          "some_directory/path",
					Exe:          "test",
					Comm:         "",
					Cmdline:      []string{"c1", "c2", "c3"},
					Uids:         []int32{1, 2, 3, 4},
					Gids:         []int32{1, 2, 3, 4, 5},
					CreationTime: time.UnixMilli(creationTime1).UTC(),
				},
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "9999",
					},
					Pid:          9999,
					Ppid:         8,
					NsPid:        3,
					Name:         "some name 9999",
					Cwd:          "some_directory/path/for",
					Exe:          "exe",
					Comm:         "something",
					Cmdline:      []string{"c1", "c2", "c3", "c4"},
					Uids:         []int32{},
					Gids:         []int32{},
					CreationTime: time.UnixMilli(creationTime2).UTC(),
				},
			},
		},
		{
			description:     "2 new processes and 2 finishes",
			configOverrides: map[string]interface{}{},
			processesToCollectA: map[int32]*procutil.Process{
				1234: {
					Pid:     1234,
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
						CreateTime:  creationTime1,
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
				},
				9999: {
					Pid:     9999,
					Ppid:    8,
					NsPid:   3,
					Name:    "some name 9999",
					Cwd:     "some_directory/path/for",
					Exe:     "exe",
					Comm:    "something",
					Cmdline: []string{"c1", "c2", "c3", "c4"},
					Uids:    []int32{},
					Gids:    []int32{},
					Stats: &procutil.Stats{
						CreateTime:  creationTime2,
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
				},
			},
			processesToCollectB:   map[int32]*procutil.Process{},
			expectedLiveProcesses: []*workloadmeta.Process{},
			expectedDeletedProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "9999",
					},
					Pid:          9999,
					Ppid:         8,
					NsPid:        3,
					Name:         "some name 9999",
					Cwd:          "some_directory/path/for",
					Exe:          "exe",
					Comm:         "something",
					Cmdline:      []string{"c1", "c2", "c3", "c4"},
					Uids:         []int32{},
					Gids:         []int32{},
					CreationTime: time.UnixMilli(creationTime2).UTC(),
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},
					Pid:          1234,
					Ppid:         6,
					NsPid:        2,
					Name:         "some name",
					Cwd:          "some_directory/path",
					Exe:          "test",
					Comm:         "",
					Cmdline:      []string{"c1", "c2", "c3"},
					Uids:         []int32{1, 2, 3, 4},
					Gids:         []int32{1, 2, 3, 4, 5},
					CreationTime: time.UnixMilli(creationTime1).UTC(),
				},
			},
		},
		{
			description:     "2 new processes, 2 finish, but 2 new processes with the same pid",
			configOverrides: map[string]interface{}{},
			processesToCollectA: map[int32]*procutil.Process{
				1234: {
					Pid:     1234,
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
						CreateTime:  creationTime1,
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
				},
				9999: {
					Pid:     9999,
					Ppid:    8,
					NsPid:   3,
					Name:    "some name 9999",
					Cwd:     "some_directory/path/for",
					Exe:     "exe",
					Comm:    "something",
					Cmdline: []string{"c1", "c2", "c3", "c4"},
					Uids:    []int32{},
					Gids:    []int32{},
					Stats: &procutil.Stats{
						CreateTime:  creationTime2,
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
				},
			},
			processesToCollectB: map[int32]*procutil.Process{
				1234: {
					Pid:     1234,
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
						CreateTime:  creationTime2,
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
				},
				9999: {
					Pid:     9999,
					Ppid:    8,
					NsPid:   3,
					Name:    "some name 9999",
					Cwd:     "some_directory/path/for",
					Exe:     "exe",
					Comm:    "something",
					Cmdline: []string{"c1", "c2", "c3", "c4"},
					Uids:    []int32{},
					Gids:    []int32{},
					Stats: &procutil.Stats{
						CreateTime:  creationTime3,
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
				},
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},
					Pid:          1234,
					Ppid:         6,
					NsPid:        2,
					Name:         "some name",
					Cwd:          "some_directory/path",
					Exe:          "test",
					Comm:         "",
					Cmdline:      []string{"c1", "c2", "c3"},
					Uids:         []int32{1, 2, 3, 4},
					Gids:         []int32{1, 2, 3, 4, 5},
					CreationTime: time.UnixMilli(creationTime2).UTC(),
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "9999",
					},
					Pid:          9999,
					Ppid:         8,
					NsPid:        3,
					Name:         "some name 9999",
					Cwd:          "some_directory/path/for",
					Exe:          "exe",
					Comm:         "something",
					Cmdline:      []string{"c1", "c2", "c3", "c4"},
					Uids:         []int32{},
					Gids:         []int32{},
					CreationTime: time.UnixMilli(creationTime3).UTC(),
				},
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "1234",
					},
					Pid:          1234,
					Ppid:         6,
					NsPid:        2,
					Name:         "some name",
					Cwd:          "some_directory/path",
					Exe:          "test",
					Comm:         "",
					Cmdline:      []string{"c1", "c2", "c3"},
					Uids:         []int32{1, 2, 3, 4},
					Gids:         []int32{1, 2, 3, 4, 5},
					CreationTime: time.UnixMilli(creationTime1).UTC(),
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "9999",
					},
					Pid:          9999,
					Ppid:         8,
					NsPid:        3,
					Name:         "some name 9999",
					Cwd:          "some_directory/path/for",
					Exe:          "exe",
					Comm:         "something",
					Cmdline:      []string{"c1", "c2", "c3", "c4"},
					Uids:         []int32{},
					Gids:         []int32{},
					CreationTime: time.UnixMilli(creationTime2).UTC(),
				},
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			c := setUpCollectorTest(t, tc.configOverrides)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of start() once we have the config file logic finished
			err := c.collector.start(ctx, c.mockStore, collectionInterval)
			assert.NoError(t, err)

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
