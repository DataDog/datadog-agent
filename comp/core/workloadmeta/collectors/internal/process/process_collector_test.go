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
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/benbjohnson/clock"
	"github.com/golang/mock/gomock"
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
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type collectorTest struct {
	collector             *collector
	probe                 *mocks.Probe
	mockConfig            config.Mock
	mockSystemProbeConfig sysprobeconfig.Mock
	mockClock             *clock.Mock
	mockStore             workloadmetamock.Mock
	mockContainerProvider *proccontainers.MockContainerProvider
}

func setUpCollectorTest(t *testing.T, configOverrides map[string]interface{}, sysProbeConfigOverrides map[string]interface{}, wlmConfigOverrides map[string]interface{}) collectorTest {
	// mock workloadmeta store
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: wlmConfigOverrides}),
		workloadmetafxmock.MockModule(workloadmeta.Params{
			AgentType: workloadmeta.NodeAgent,
		}),
	))

	// mock container provider for container data
	mockCtrl := gomock.NewController(t)
	mockContainerProvider := proccontainers.NewMockContainerProvider(mockCtrl)

	mockClock := clock.NewMock()
	mockProbe := mocks.NewProbe(t)

	// mock config
	mockConfig := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: configOverrides}),
	))

	// mock language detection system probe config
	mockSystemProbeConfig := fxutil.Test[sysprobeconfig.Component](t, fx.Options(
		sysprobeconfigimpl.MockModule(),
		fx.Replace(config.MockParams{Overrides: sysProbeConfigOverrides}),
	))
	processCollector := newProcessCollector(collectorID, workloadmeta.NodeAgent, mockClock, mockProbe, mockConfig, mockSystemProbeConfig)
	processCollector.sysProbeClient = &http.Client{}

	return collectorTest{&processCollector, mockProbe, mockConfig, mockSystemProbeConfig, mockClock, mockStore, mockContainerProvider}
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

// TestBasicCreatedProcessesCollection tests the collector capturing new processes without language + container data
func TestBasicCreatedProcessesCollection(t *testing.T) {
	collectionInterval := time.Second * 10

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
			c := setUpCollectorTest(t, nil, nil, nil)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of 3 lines below when configuration is sorted as Start() is currently
			// by default disabled
			c.collector.containerProvider = c.mockContainerProvider
			c.collector.store = c.mockStore
			go c.collector.collectProcesses(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(1)

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

// TestCreatedProcessesCollectionWithLanguages tests the collector capturing new processes with language data
func TestCreatedProcessesCollectionWithLanguages(t *testing.T) {
	collectionInterval := time.Second * 10
	configEnableLanguageDetection := map[string]interface{}{
		"language_detection.enabled": true,
	}

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
		configOverrides    map[string]interface{}
		processesToCollect map[int32]*procutil.Process
		expectedProcesses  map[int32]*workloadmeta.Process
	}{
		{
			description:     "single new python process",
			configOverrides: configEnableLanguageDetection,
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
			description:     "multiple new processes",
			configOverrides: configEnableLanguageDetection,
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
			c := setUpCollectorTest(t, tc.configOverrides, nil, nil)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of 3 lines below when configuration is sorted as Start() is currently
			// by default disabled
			c.collector.containerProvider = c.mockContainerProvider
			c.collector.store = c.mockStore
			go c.collector.collectProcesses(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(nil).Times(1)

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

// TestCreatedProcessesCollectionWithContainers tests the collector capturing new processes with container data
func TestCreatedProcessesCollectionWithContainers(t *testing.T) {
	collectionInterval := time.Second * 10

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
			c := setUpCollectorTest(t, nil, nil, nil)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of 3 lines below when configuration is sorted as Start() is currently
			// by default disabled
			c.collector.containerProvider = c.mockContainerProvider
			c.collector.store = c.mockStore
			go c.collector.collectProcesses(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollect, nil).Times(1)

			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(tc.pidToCidToCollect).Times(1)

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

// TestCreatedProcessesCollection tests the collector capturing lifecycle of a process (creation, deletion) with all types of data
func TestProcessLifecycleCollection(t *testing.T) {
	collectionInterval := time.Second * 10
	configEnableLanguageDetection := map[string]interface{}{
		"language_detection.enabled": true,
	}
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

	for _, tc := range []struct {
		description              string
		configOverrides          map[string]interface{}
		processesToCollectA      map[int32]*procutil.Process
		pidToCidToCollectA       map[int]string
		processesToCollectB      map[int32]*procutil.Process
		pidToCidToCollectB       map[int]string
		expectedDeletedProcesses []*workloadmeta.Process
		expectedLiveProcesses    []*workloadmeta.Process
	}{
		{
			description:     "2 new processes and 1 finishes",
			configOverrides: configEnableLanguageDetection,
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
			description:     "2 new processes and 2 finishes",
			configOverrides: configEnableLanguageDetection,
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
			description:     "2 new processes, 2 finish, but 2 new processes with the same pid",
			configOverrides: configEnableLanguageDetection,
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
			description:     "2 new processes with containers, 2 finish, but 2 new processes with the same pid diff container",
			configOverrides: configEnableLanguageDetection,
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
	} {
		t.Run(tc.description, func(t *testing.T) {
			c := setUpCollectorTest(t, tc.configOverrides, nil, nil)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// TODO: we should use Start() instead of 3 lines below when configuration is sorted as Start() is currently
			// by default disabled
			c.collector.containerProvider = c.mockContainerProvider
			c.collector.store = c.mockStore
			go c.collector.collectProcesses(ctx, c.collector.clock.Ticker(collectionInterval))
			go c.collector.stream(ctx)

			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollectA, nil).Times(1)
			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(tc.pidToCidToCollectA).Times(1)

			// update clock to trigger processing
			c.mockClock.Add(collectionInterval)
			c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(tc.processesToCollectB, nil).Times(1)

			c.mockContainerProvider.EXPECT().GetPidToCid(cacheValidityNoRT).Return(tc.pidToCidToCollectB).Times(1)
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
