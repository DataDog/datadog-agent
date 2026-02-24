// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package process implements the process collector for Workloadmeta.
package process

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type collectorTest struct {
	collector             *collector
	probe                 *mocks.Probe
	mockSystemProbeConfig sysprobeconfig.Mock
	mockClock             *clock.Mock
	mockStore             workloadmetamock.Mock
	mockContainerProvider *proccontainers.MockContainerProvider
}

func (c collectorTest) cleanup() {
	// when service discovery is enabled, we need to reset the global telemetry registry
	// since the start function registers a new gauge every time that errors
	telemetry.GetCompatComponent().Reset()
}

// TestBasicCreatedProcessesCollection tests the collector capturing new processes without language + container data
func TestBasicCreatedProcessesCollection(t *testing.T) {
	creationTime1 := time.Now().Unix()
	pid1 := int32(1234)
	proc1 := createTestPythonProcess(pid1, creationTime1)

	creationTime2 := time.Now().Add(time.Second).Unix()
	pid2 := int32(9999)
	proc2 := createTestJavaProcess(pid2, creationTime2)

	creationTime3 := time.Now().Add(time.Second).Unix()
	pid3 := int32(3333)
	proc3 := createTestUnknownProcess(pid3, creationTime3)

	for _, tc := range []struct {
		description        string
		processesToCollect map[int32]*procutil.Process
		expectedProcesses  map[int32]*workloadmeta.Process
	}{
		{
			description: "single new python process",
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				proc1.Pid: workloadmetaProcess(proc1, nil, nil),
			},
		},
		{
			description: "multiple new processes",
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
				proc3.Pid: proc3,
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				proc1.Pid: workloadmetaProcess(proc1, nil, nil),
				proc2.Pid: workloadmetaProcess(proc2, nil, nil),
				proc3.Pid: workloadmetaProcess(proc3, nil, nil),
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetInTest("process_config.process_collection.enabled", true)
			cfg.SetInTest("process_config.intervals.process", 10)

			c := setUpCollectorTest(t, cfg, nil, nil)
			defer c.cleanup()
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(1)

			// start execution
			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

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

// TestCreatedProcessesCollectionWithLanguages tests the collector capturing new processes with language data
func TestCreatedProcessesCollectionWithLanguages(t *testing.T) {
	creationTime1 := time.Now().Unix()
	pid1 := int32(1234)
	proc1 := createTestPythonProcess(pid1, creationTime1)

	creationTime2 := time.Now().Add(time.Second).Unix()
	pid2 := int32(9999)
	proc2 := createTestJavaProcess(pid2, creationTime2)

	creationTime3 := time.Now().Add(time.Second).Unix()
	pid3 := int32(3333)
	proc3 := createTestUnknownProcess(pid3, creationTime3)

	for _, tc := range []struct {
		description        string
		processesToCollect map[int32]*procutil.Process
		expectedProcesses  map[int32]*workloadmeta.Process
	}{
		{
			description: "single new python process",
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				proc1.Pid: workloadmetaProcess(proc1, &languagemodels.Language{
					Name: languagemodels.Python,
				}, nil),
			},
		},
		{
			description: "multiple new processes",
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
				proc3.Pid: proc3,
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				proc1.Pid: workloadmetaProcess(proc1, &languagemodels.Language{
					Name: languagemodels.Python,
				}, nil),
				proc2.Pid: workloadmetaProcess(proc2, &languagemodels.Language{
					Name: languagemodels.Java,
				}, nil),
				proc3.Pid: workloadmetaProcess(proc3, &languagemodels.Language{
					Name: languagemodels.Unknown,
				}, nil),
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetInTest("process_config.process_collection.enabled", true)
			cfg.SetInTest("process_config.intervals.process", 10)
			cfg.SetInTest("language_detection.enabled", true)

			c := setUpCollectorTest(t, cfg, nil, nil)
			defer c.cleanup()
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(1)

			// start execution
			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

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

// TestCreatedProcessesCollectionWithContainers tests the collector capturing new processes with container data
func TestCreatedProcessesCollectionWithContainers(t *testing.T) {
	creationTime1 := time.Now().Unix()
	pid1 := int32(1234)
	proc1 := createTestPythonProcess(pid1, creationTime1)

	creationTime2 := time.Now().Add(time.Second).Unix()
	pid2 := int32(9999)
	proc2 := createTestJavaProcess(pid2, creationTime2)

	creationTime3 := time.Now().Add(time.Second * 2).Unix()
	pid3 := int32(3333)
	proc3 := createTestJavaProcess(pid3, creationTime3)

	creationTime4 := time.Now().Add(time.Second * 3).Unix()
	pid4 := int32(4444)
	proc4 := createTestUnknownProcess(pid4, creationTime4)

	for _, tc := range []struct {
		description        string
		processesToCollect map[int32]*procutil.Process
		pidToCidToCollect  map[int]string
		expectedProcesses  map[int32]*workloadmeta.Process
	}{
		{
			description: "single new python process in a container",
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
			},
			pidToCidToCollect: map[int]string{
				int(proc1.Pid): "some_container_id",
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				proc1.Pid: workloadmetaProcess(proc1, nil,
					&workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "some_container_id",
					}),
			},
		},
		{
			description: "multiple new processes in containers",
			processesToCollect: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
				proc3.Pid: proc3,
				proc4.Pid: proc4,
			},
			pidToCidToCollect: map[int]string{
				int(proc1.Pid): "some_container_id1",
				int(proc2.Pid): "some_container_id2",
				int(proc3.Pid): "some_container_id2",
				int(proc4.Pid): "some_container_id4",
			},
			expectedProcesses: map[int32]*workloadmeta.Process{
				proc1.Pid: workloadmetaProcess(proc1, nil,
					&workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "some_container_id1",
					}),
				proc2.Pid: workloadmetaProcess(proc2, nil,
					&workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "some_container_id2",
					}),
				proc3.Pid: workloadmetaProcess(proc3, nil,
					&workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "some_container_id2",
					}),
				proc4.Pid: workloadmetaProcess(proc4, nil,
					&workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "some_container_id4",
					}),
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetInTest("process_config.process_collection.enabled", true)
			cfg.SetInTest("process_config.intervals.process", 10)

			c := setUpCollectorTest(t, cfg, nil, nil)
			defer c.cleanup()
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(tc.pidToCidToCollect).Times(1)

			// start execution
			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

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

// TestCreatedProcessesCollection tests the collector capturing lifecycle of a process (creation, deletion) with all types of data
func TestProcessLifecycleCollection(t *testing.T) {
	collectionInterval := time.Second * 10
	creationTime1 := time.Now().Unix()
	pid1 := int32(1234)
	proc1 := createTestPythonProcess(pid1, creationTime1)

	creationTime2 := time.Now().Add(time.Second).Unix()
	pid2 := int32(9999)
	proc2 := createTestJavaProcess(pid2, creationTime2)

	// same pid as proc1 but different creation time
	proc3 := createTestPythonProcess(pid1, creationTime2)

	// same pid as proc2 but different creation time and unknown language
	creationTime3 := time.Now().Add(2 * time.Second).Unix()
	proc4 := createTestUnknownProcess(pid2, creationTime3)

	// same pid AND same creation time as proc1, but different cmdline
	proc1DiffCmdline := createTestJavaProcess(pid1, creationTime1)

	for _, tc := range []struct {
		description              string
		processesToCollectA      map[int32]*procutil.Process
		pidToCidToCollectA       map[int]string
		processesToCollectB      map[int32]*procutil.Process
		pidToCidToCollectB       map[int]string
		expectedDeletedProcesses []*workloadmeta.Process
		expectedLiveProcesses    []*workloadmeta.Process
	}{
		{
			description: "2 new processes and 1 finishes",
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			processesToCollectB: map[int32]*procutil.Process{
				proc2.Pid: proc2,
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc1,
					nil,
					nil),
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc2,
					&languagemodels.Language{
						Name: languagemodels.Java,
					},
					nil),
			},
		},
		{
			description: "2 new processes and 2 finishes",
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			processesToCollectB:   map[int32]*procutil.Process{},
			expectedLiveProcesses: []*workloadmeta.Process{},
			expectedDeletedProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc1, nil, nil),
				workloadmetaProcess(proc2, nil, nil),
			},
		},
		{
			description: "2 new processes, 2 finish, but 2 new processes with the same pid",
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			processesToCollectB: map[int32]*procutil.Process{
				proc3.Pid: proc3,
				proc4.Pid: proc4,
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc3,
					&languagemodels.Language{
						Name: languagemodels.Python,
					},
					nil),
				workloadmetaProcess(proc4,
					&languagemodels.Language{
						Name: languagemodels.Unknown,
					}, nil),
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc1, nil, nil),
				workloadmetaProcess(proc2, nil, nil),
			},
		},
		{
			description: "2 new processes with containers, 2 finish, but 2 new processes with the same pid diff container",
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1,
				proc2.Pid: proc2,
			},
			pidToCidToCollectA: map[int]string{
				int(proc1.Pid): "container_id_1",
				int(proc2.Pid): "container_id_2",
			},
			processesToCollectB: map[int32]*procutil.Process{
				proc3.Pid: proc3,
				proc4.Pid: proc4,
			},
			pidToCidToCollectB: map[int]string{
				int(proc3.Pid): "container_id_3",
				int(proc4.Pid): "container_id_4",
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc3,
					&languagemodels.Language{
						Name: languagemodels.Python,
					},
					&workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container_id_3",
					}),
				workloadmetaProcess(proc4,
					&languagemodels.Language{
						Name: languagemodels.Unknown,
					},
					&workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container_id_4",
					}),
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc1, nil, nil),
				workloadmetaProcess(proc2, nil, nil),
			},
		},
		{
			description: "process exec: same pid, same createTime, different cmdline",
			processesToCollectA: map[int32]*procutil.Process{
				proc1.Pid: proc1, // python process
			},
			processesToCollectB: map[int32]*procutil.Process{
				proc1DiffCmdline.Pid: proc1DiffCmdline, // java process with same pid and createTime
			},
			expectedLiveProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc1DiffCmdline,
					&languagemodels.Language{
						Name: languagemodels.Java,
					},
					nil),
			},
			expectedDeletedProcesses: []*workloadmeta.Process{
				workloadmetaProcess(proc1, nil, nil),
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetInTest("process_config.process_collection.enabled", true)
			cfg.SetInTest("process_config.intervals.process", 10)
			cfg.SetInTest("language_detection.enabled", true)

			c := setUpCollectorTest(t, cfg, nil, nil)
			defer c.cleanup()
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollectA, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(tc.pidToCidToCollectA).Times(1)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollectB, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(tc.pidToCidToCollectB).Times(1)

			// start execution
			err := c.collector.Start(ctx, c.mockStore)
			assert.NoError(t, err)

			// update clock to trigger second collection
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

					// the same process pid can exist so we ensure it is a different process using ProcessIdentity
					if exists {
						expectedIdentity := procutil.ProcessIdentity(expectedDeletedProc.Pid, expectedDeletedProc.CreationTime.UnixMilli(), expectedDeletedProc.Cmdline)
						actualIdentity := procutil.ProcessIdentity(actualProc.Pid, actualProc.CreationTime.UnixMilli(), actualProc.Cmdline)
						assert.NotEqual(cT, expectedIdentity, actualIdentity, "Expected process to be replaced")
					}

				}
			}, time.Second, time.Millisecond*100)
		})
	}
}

func TestStartConfiguration(t *testing.T) {
	for _, tc := range []struct {
		description        string
		configOverrides    map[string]interface{}
		sysConfigOverrides map[string]interface{}
		expectedError      error
	}{
		{
			description: "everything enabled correctly",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": true,
			},
			sysConfigOverrides: map[string]interface{}{
				"discovery.enabled": true,
			},
			expectedError: nil,
		},
		{
			description: "only process collection enabled",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": true,
			},
			sysConfigOverrides: map[string]interface{}{
				"discovery.enabled": false,
			},
			expectedError: nil,
		},
		{
			description: "only service discovery enabled",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": false,
			},
			sysConfigOverrides: map[string]interface{}{
				"discovery.enabled": true,
			},
			expectedError: nil,
		},
		{
			description: "only GPU monitoring enabled",
			configOverrides: map[string]interface{}{
				"gpu.enabled": true,
			},
			sysConfigOverrides: map[string]interface{}{
				"discovery.enabled": false,
			},
			expectedError: nil,
		},
		{
			description: "process collection and service discovery not enabled",
			configOverrides: map[string]interface{}{
				"process_config.process_collection.enabled": false,
			},
			sysConfigOverrides: map[string]interface{}{
				"discovery.enabled": false,
			},
			expectedError: errors.NewDisabled(componentName, "process collection, service discovery, language collection, and GPU monitoring are disabled"),
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			cfg := config.NewMock(t)
			for k, v := range tc.configOverrides {
				cfg.SetInTest(k, v)
			}

			c := setUpCollectorTest(t, cfg, tc.sysConfigOverrides, nil)
			defer c.cleanup()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// set up mocks as some configurations result in calls
			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(map[int32]*procutil.Process{}, nil).Maybe()
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(map[int]string{}).AnyTimes()

			err := c.collector.Start(ctx, c.mockStore)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}

func TestProcessCollectorIntervalConfig(t *testing.T) {
	for _, tc := range []struct {
		description      string
		intervalProcess  int
		expectedInterval time.Duration
	}{
		{
			description:      "unconfigured defaults",
			expectedInterval: 10 * time.Second,
		},
		{
			description:      "shorter than default config",
			intervalProcess:  5,
			expectedInterval: 5 * time.Second,
		},
		{
			description:      "longer than default config but shorter than service discovery interval",
			intervalProcess:  30,
			expectedInterval: 30 * time.Second,
		},
		{
			description:      "longer than service discovery interval fallback to max interval",
			intervalProcess:  61,
			expectedInterval: 60 * time.Second,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			cfg := config.NewMock(t)
			if tc.intervalProcess != 0 {
				cfg.SetInTest("process_config.intervals.process", tc.intervalProcess)
			}

			c := setUpCollectorTest(t, cfg, nil, nil)
			actualInterval := c.collector.processCollectionIntervalConfig()
			assert.Equal(t, tc.expectedInterval, actualInterval)
		})
	}
}

// TestProcessDifferentCmdline tests that the full collector flow correctly handles
// different cmdline scenarios (same PID, same createTime, but different cmdline).
func TestProcessDifferentCmdline(t *testing.T) {
	collectionInterval := time.Second * 10
	createTime := time.Now().Unix()
	pid := int32(1234)

	// First collection: bash process
	bashProc := &procutil.Process{
		Pid:     pid,
		Ppid:    6,
		NsPid:   2,
		Name:    "bash",
		Cwd:     "/home/user",
		Exe:     "/bin/bash",
		Comm:    "bash",
		Cmdline: []string{"bash"},
		Uids:    []int32{1000},
		Gids:    []int32{1000},
		Stats:   &procutil.Stats{CreateTime: createTime},
	}

	// Second collection: htop (same PID and createTime, simulating exec)
	htopProc := &procutil.Process{
		Pid:     pid,
		Ppid:    6,
		NsPid:   2,
		Name:    "htop",
		Cwd:     "/home/user",
		Exe:     "/usr/bin/htop",
		Comm:    "htop",
		Cmdline: []string{"htop"},
		Uids:    []int32{1000},
		Gids:    []int32{1000},
		Stats:   &procutil.Stats{CreateTime: createTime}, // Same createTime!
	}

	cfg := config.NewMock(t)
	cfg.SetInTest("process_config.process_collection.enabled", true)
	cfg.SetInTest("process_config.intervals.process", 10)
	cfg.SetInTest("language_detection.enabled", true)

	c := setUpCollectorTest(t, cfg, nil, nil)
	defer c.cleanup()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// First collection returns bash
	c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(map[int32]*procutil.Process{pid: bashProc}, nil).Times(1)
	c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(1)

	// Second collection returns htop (exec'd)
	c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(map[int32]*procutil.Process{pid: htopProc}, nil).Times(1)
	c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(1)

	// Start collection
	err := c.collector.Start(ctx, c.mockStore)
	assert.NoError(t, err)

	// Wait for first collection to complete
	assert.EventuallyWithT(t, func(cT *assert.CollectT) {
		actualProc, err := c.mockStore.GetProcess(pid)
		if !assert.NoError(cT, err) || !assert.NotNil(cT, actualProc) {
			return
		}
		assert.Equal(cT, []string{"bash"}, actualProc.Cmdline)
	}, time.Second, time.Millisecond*100)

	// Trigger second collection
	c.mockClock.Add(collectionInterval)

	// After exec, the store should have htop, not bash
	assert.EventuallyWithT(t, func(cT *assert.CollectT) {
		actualProc, err := c.mockStore.GetProcess(pid)
		if !assert.NoError(cT, err) || !assert.NotNil(cT, actualProc) {
			return
		}
		// Critical assertion: cmdline should be updated to htop after exec
		assert.Equal(cT, []string{"htop"}, actualProc.Cmdline, "Process cmdline should be updated after exec")
		assert.Equal(cT, "htop", actualProc.Name, "Process name should be updated after exec")
	}, time.Second, time.Millisecond*100)
}

// TestProcessCacheDifferentCmdline tests that processCacheDifference correctly detects
// when a process has a different cmdline (same PID, same CreateTime, different Cmdline).
func TestProcessCacheDifferentCmdline(t *testing.T) {
	createTime := time.Now().Unix()
	pid := int32(12345)

	// Original bash process
	bashProc := &procutil.Process{
		Pid:     pid,
		Cmdline: []string{"bash"},
		Stats:   &procutil.Stats{CreateTime: createTime},
	}

	// Same PID and createTime, but exec'd into htop
	htopProc := &procutil.Process{
		Pid:     pid,
		Cmdline: []string{"htop"},
		Stats:   &procutil.Stats{CreateTime: createTime},
	}

	// Cache A has htop (current state after exec)
	cacheA := map[int32]*procutil.Process{
		pid: htopProc,
	}

	// Cache B has bash (previous state before exec)
	cacheB := map[int32]*procutil.Process{
		pid: bashProc,
	}

	// processCacheDifference(A, B) should return htop as a "new" process because cmdline changed
	diff := processCacheDifference(cacheA, cacheB)

	assert.Len(t, diff, 1, "Expected one process in diff after exec cmdline change")
	assert.Equal(t, pid, diff[0].Pid)
	assert.Equal(t, []string{"htop"}, diff[0].Cmdline)
}

// TestProcessCacheSameCmdline tests that processCacheDifference
// does not report a process as new when the cmdline stays the same.
func TestProcessCacheSameCmdline(t *testing.T) {
	createTime := time.Now().Unix()
	pid := int32(12345)

	procA := &procutil.Process{
		Pid:     pid,
		Cmdline: []string{"bash"},
		Stats:   &procutil.Stats{CreateTime: createTime},
	}

	procB := &procutil.Process{
		Pid:     pid,
		Cmdline: []string{"bash"},
		Stats:   &procutil.Stats{CreateTime: createTime},
	}

	cacheA := map[int32]*procutil.Process{
		pid: procA,
	}

	cacheB := map[int32]*procutil.Process{
		pid: procB,
	}

	diff := processCacheDifference(cacheA, cacheB)

	assert.Len(t, diff, 0, "Expected no processes in diff when cmdline is the same")
}

func setUpCollectorTest(t *testing.T, cfg config.Component, sysProbeConfigOverrides map[string]interface{}, wlmConfigOverrides map[string]interface{}) collectorTest {
	// mock workloadmeta store
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component { return config.NewMockWithOverrides(t, wlmConfigOverrides) }),
		workloadmetafxmock.MockModule(workloadmeta.Params{
			AgentType: workloadmeta.NodeAgent,
		}),
	))

	// mock container provider for container data
	mockCtrl := gomock.NewController(t)
	mockContainerProvider := proccontainers.NewMockContainerProvider(mockCtrl)

	mockClock := clock.NewMock()
	mockProbe := mocks.NewProbe(t)

	// mock language detection system probe config
	mockSystemProbeConfig := fxutil.Test[sysprobeconfig.Component](t, fx.Options(
		sysprobeconfigimpl.MockModule(),
		fx.Replace(sysprobeconfigimpl.MockParams{Overrides: sysProbeConfigOverrides}),
	))
	processCollector := newProcessCollector(collectorID, workloadmeta.NodeAgent, mockClock, mockProbe, cfg, mockSystemProbeConfig)
	processCollector.containerProvider = mockContainerProvider

	return collectorTest{&processCollector, mockProbe, mockSystemProbeConfig, mockClock, mockStore, mockContainerProvider}
}

func createTestPythonProcess(pid int32, createTime int64) *procutil.Process {
	proc := &procutil.Process{
		Pid:     pid,
		Ppid:    6,
		NsPid:   2,
		Name:    "some name",
		Cwd:     "some_directory/path",
		Exe:     "test",
		Comm:    "",
		Cmdline: []string{"python3", "--version"},
		Uids:    []int32{1, 2, 3, 4},
		Gids:    []int32{1, 2, 3, 4, 5},
		Stats: &procutil.Stats{
			CreateTime: createTime,
		},
	}
	return proc
}

func createTestJavaProcess(pid int32, createTime int64) *procutil.Process {
	proc := &procutil.Process{
		Pid:     pid,
		Ppid:    9,
		NsPid:   3,
		Name:    "some name 2",
		Cwd:     "some_directory/path/path2",
		Exe:     "exe",
		Comm:    "hello",
		Cmdline: []string{"java", "-c", "org.elasticsearch.bootstrap.Elasticsearch"},
		Uids:    []int32{1},
		Gids:    []int32{1, 2},
		Stats: &procutil.Stats{
			CreateTime: createTime,
		},
	}

	return proc
}

func createTestUnknownProcess(pid int32, createTime int64) *procutil.Process {
	proc := &procutil.Process{
		Pid:     pid,
		Ppid:    3,
		NsPid:   8,
		Name:    "some name 3",
		Cwd:     "some_directory/path/path3",
		Exe:     "",
		Comm:    "?",
		Cmdline: []string{"something_unknown", "-p", "8080"},
		Uids:    []int32{50, 1},
		Gids:    []int32{20, 30},
		Stats: &procutil.Stats{
			CreateTime: createTime,
		},
	}

	return proc
}

func workloadmetaProcess(proc *procutil.Process, language *languagemodels.Language, owner *workloadmeta.EntityID) *workloadmeta.Process {
	// setting ContainerID since it is existing behaviour, but will be eventually removed once we only use the Owner field
	cid := ""
	if owner != nil {
		cid = owner.ID
	}
	wlmProc := &workloadmeta.Process{
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
		Language:     language,
		ContainerID:  cid,
		Owner:        owner,
	}
	return wlmProc
}
