// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package process implements the local process collector for
// Workloadmeta.
package process

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	processwlm "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// NewProcessDataWithMockProbe returns a new ProcessData with a mock probe
func NewProcessDataWithMockProbe(t *testing.T) (*Data, *mocks.Probe) {
	probe := mocks.NewProbe(t)
	return &Data{
		probe: probe,
	}, probe
}

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

	wlmExtractor := processwlm.NewWorkloadMetaExtractor(mockStore.GetConfig())
	mockProcessData, probe := NewProcessDataWithMockProbe(t)
	mockProcessData.Register(wlmExtractor)
	mockClock := clock.NewMock()
	mockCtrl := gomock.NewController(t)
	mockProvider := proccontainers.NewMockContainerProvider(mockCtrl)
	processDiffCh := wlmExtractor.ProcessCacheDiff()
	processCollector := &collector{
		id:                collectorID,
		store:             mockStore,
		catalog:           workloadmeta.NodeAgent,
		processDiffCh:     processDiffCh,
		processData:       mockProcessData,
		pidToCid:          make(map[int]string),
		wlmExtractor:      wlmExtractor,
		collectionClock:   mockClock,
		containerProvider: mockProvider,
	}

	return collectorTest{processCollector, probe, mockClock, mockStore}
}

func TestProcessCollector(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.DefaultAgent)

	configOverrides := map[string]interface{}{
		"language_detection.enabled":                true,
		"process_config.process_collection.enabled": true,
		"process_config.run_in_core_agent.enabled":  true,
	}

	c := setUpCollectorTest(t, configOverrides)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	processDiffCh := make(chan *processwlm.ProcessCacheDiff)
	c.collector.processDiffCh = processDiffCh

	err := c.collector.Start(ctx, c.mockStore)
	require.NoError(t, err)

	creationTime := time.Now().Unix()
	processDiffCh <- &processwlm.ProcessCacheDiff{
		Creation: []*processwlm.ProcessEntity{
			{
				Pid:          1,
				ContainerId:  "cid",
				NsPid:        1,
				CreationTime: creationTime,
				Language:     &languagemodels.Language{Name: languagemodels.Java},
			},
		},
	}

	expectedProc1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			ID:   "1",
			Kind: workloadmeta.KindProcess,
		},
		NsPid:        1,
		ContainerID:  "cid",
		CreationTime: time.UnixMilli(creationTime),
		Language:     &languagemodels.Language{Name: languagemodels.Java},
	}

	assert.EventuallyWithT(t, func(cT *assert.CollectT) {
		proc, err := c.mockStore.GetProcess(1)
		assert.NoError(cT, err)
		assert.Equal(cT, expectedProc1, proc)
	}, time.Second, time.Millisecond*100)

	processDiffCh <- &processwlm.ProcessCacheDiff{
		Creation: []*processwlm.ProcessEntity{
			{
				Pid:          2,
				ContainerId:  "cid",
				NsPid:        2,
				CreationTime: creationTime,
				Language:     &languagemodels.Language{Name: languagemodels.Python},
			},
		},
		Deletion: []*processwlm.ProcessEntity{
			{
				Pid:          1,
				ContainerId:  "cid",
				NsPid:        1,
				CreationTime: creationTime,
				Language:     &languagemodels.Language{Name: languagemodels.Java},
			},
		},
	}

	expectedProc2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			ID:   "2",
			Kind: workloadmeta.KindProcess,
		},
		NsPid:        2,
		ContainerID:  "cid",
		CreationTime: time.UnixMilli(creationTime),
		Language:     &languagemodels.Language{Name: languagemodels.Python},
	}

	assert.EventuallyWithT(t, func(cT *assert.CollectT) {
		proc, err := c.mockStore.GetProcess(2)
		assert.NoError(cT, err)
		assert.Equal(cT, expectedProc2, proc)

		_, err = c.mockStore.GetProcess(1)
		assert.Error(cT, err)
	}, time.Second, time.Millisecond*100)
}

func TestProcessCollectorStart(t *testing.T) {
	tests := []struct {
		name                 string
		agentFlavor          string
		langDetectionEnabled bool
		runInCoreAgent       bool
		expectedEnabled      bool
	}{
		{
			name:                 "core agent + all configs enabled",
			agentFlavor:          flavor.DefaultAgent,
			langDetectionEnabled: true,
			runInCoreAgent:       true,
			expectedEnabled:      true,
		},
		{
			name:                 "core agent + all configs disabled",
			agentFlavor:          flavor.DefaultAgent,
			langDetectionEnabled: false,
			runInCoreAgent:       false,
			expectedEnabled:      false,
		},
		{
			name:                 "process agent + all configs enabled",
			agentFlavor:          flavor.ProcessAgent,
			langDetectionEnabled: true,
			runInCoreAgent:       true,
			expectedEnabled:      false,
		},
		{
			name:                 "process agent + all configs disabled",
			agentFlavor:          flavor.ProcessAgent,
			langDetectionEnabled: false,
			runInCoreAgent:       false,
			expectedEnabled:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalFlavor := flavor.GetFlavor()
			defer flavor.SetFlavor(originalFlavor)
			flavor.SetFlavor(test.agentFlavor)

			configOverrides := map[string]interface{}{
				"language_detection.enabled":               test.langDetectionEnabled,
				"process_config.run_in_core_agent.enabled": test.runInCoreAgent,
			}

			c := setUpCollectorTest(t, configOverrides)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			err := c.collector.Start(ctx, c.mockStore)

			enabled := err == nil
			assert.Equal(t, test.expectedEnabled, enabled)
		})
	}
}

func TestProcessCollectorWithoutProcessCheck(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.DefaultAgent)

	configOverrides := map[string]interface{}{
		"language_detection.enabled":                true,
		"process_config.process_collection.enabled": false,
		"process_config.run_in_core_agent.enabled":  true,
	}

	c := setUpCollectorTest(t, configOverrides)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	mockProvider := proccontainers.NewMockContainerProvider(mockCtrl)
	c.collector.containerProvider = mockProvider

	err := c.collector.Start(ctx, c.mockStore)
	require.NoError(t, err)

	c.probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(map[int32]*procutil.Process{
		1: {
			Pid:     1,
			Cmdline: []string{"proc", "-h", "-v"},
			Stats:   &procutil.Stats{CreateTime: 1},
		},
	}, nil).Times(1)

	// Testing container id enrichment
	expectedCid := "container1"
	mockProvider.EXPECT().GetPidToCid(2 * time.Second).Return(map[int]string{1: expectedCid}).MinTimes(1)

	c.mockClock.Add(10 * time.Second)

	assert.EventuallyWithT(t, func(cT *assert.CollectT) {
		proc, err := c.mockStore.GetProcess(1)
		assert.NoError(cT, err)
		assert.NotNil(cT, proc)
		assert.Equal(cT, expectedCid, proc.ContainerID)
	}, 1*time.Second, time.Millisecond*100)
}
