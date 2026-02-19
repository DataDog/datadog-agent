// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	collectormock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type fakeCIDProvider struct {
	entries     map[string]string
	initEntries map[string]string
}

func (f *fakeCIDProvider) ContainerIDForPodUIDAndContName(podUID, contName string, initCont bool, _ time.Duration) (string, error) {
	id := podUID + "/" + contName
	if initCont {
		return f.initEntries[id], nil
	}
	return f.entries[id], nil
}

// Sets up the fake metrics provider and returns a function to reset the original metrics provider
func setupFakeMetricsProvider(mockMetricsProvider metrics.Provider) func() {
	originalMetricsProvider := metrics.GetProvider
	metrics.GetProvider = func(_ option.Option[workloadmeta.Component]) metrics.Provider {
		return mockMetricsProvider
	}
	return func() { metrics.GetProvider = originalMetricsProvider }
}

func TestTag(t *testing.T) {
	entityID := types.NewEntityID(types.ContainerID, "123")

	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp
	tagStore := fakeTagger.GetTagStore()

	tagStore.ProcessTagInfo([]*types.TagInfo{
		{
			EntityID:             entityID,
			Source:               "stream",
			LowCardTags:          []string{"low1"},
			OrchestratorCardTags: []string{"orchestrator1"},
			HighCardTags:         []string{"high1"},
		},
		{
			EntityID:             entityID,
			Source:               "pull",
			LowCardTags:          []string{"low2"},
			OrchestratorCardTags: []string{"orchestrator2"},
			HighCardTags:         []string{"high2"},
		},
	})

	noneCardTags, err := fakeTagger.Tag(entityID, types.NoneCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{}, noneCardTags)

	lowCardTags, err := fakeTagger.Tag(entityID, types.LowCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2"}, lowCardTags)

	orchestratorCardTags, err := fakeTagger.Tag(entityID, types.OrchestratorCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2", "orchestrator1", "orchestrator2"}, orchestratorCardTags)

	highCardTags, err := fakeTagger.Tag(entityID, types.HighCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2", "orchestrator1", "orchestrator2", "high1", "high2"}, highCardTags)

	undefinedTags, err := fakeTagger.Tag(types.NewEntityID(types.ContainerID, "undefined-entity"), types.HighCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{}, undefinedTags)
}

func TestTagWithCompleteness(t *testing.T) {
	completeEntityID := types.NewEntityID(types.ContainerID, "complete")
	incompleteEntityID := types.NewEntityID(types.ContainerID, "incomplete")
	missingEntityID := types.NewEntityID(types.ContainerID, "missing")

	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	testTagger := NewMock(mockReq).Comp
	tagStore := testTagger.GetTagStore()

	tagStore.ProcessTagInfo([]*types.TagInfo{
		{
			Source:      "source",
			EntityID:    completeEntityID,
			LowCardTags: []string{"low"},
			IsComplete:  true,
		},
		{
			Source:      "source",
			EntityID:    incompleteEntityID,
			LowCardTags: []string{"low"},
			IsComplete:  false,
		},
	})

	for _, test := range []struct {
		name               string
		entityID           types.EntityID
		expectedTags       []string
		expectedIsComplete bool
	}{
		{
			name:               "complete entity",
			entityID:           completeEntityID,
			expectedTags:       []string{"low"},
			expectedIsComplete: true,
		},
		{
			name:               "incomplete entity",
			entityID:           incompleteEntityID,
			expectedTags:       []string{"low"},
			expectedIsComplete: false,
		},
		{
			name:               "entity not found",
			entityID:           missingEntityID,
			expectedTags:       []string{},
			expectedIsComplete: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tags, isComplete, err := testTagger.TagWithCompleteness(test.entityID, types.LowCardinality)
			assert.NoError(t, err)
			assert.ElementsMatch(t, test.expectedTags, tags)
			assert.Equal(t, test.expectedIsComplete, isComplete)
		})
	}
}

func TestGenerateContainerIDFromOriginInfo(t *testing.T) {
	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp

	// Overriding the GetProvider function to use the mock metrics provider
	mockMetricsProvider := collectormock.NewMetricsProvider()
	cleanUp := setupFakeMetricsProvider(mockMetricsProvider)
	defer cleanUp()

	for _, tt := range []struct {
		name                string
		originInfo          origindetection.OriginInfo
		expectedContainerID string
		expectedError       error
		setup               func()
	}{
		{
			name:                "with empty OriginInfo",
			originInfo:          origindetection.OriginInfo{},
			expectedContainerID: "",
			expectedError:       fmt.Errorf("unable to resolve container ID from OriginInfo: %+v", origindetection.OriginInfo{}),
			setup:               func() {},
		},
		{
			name: "with container ID",
			originInfo: origindetection.OriginInfo{
				LocalData: origindetection.LocalData{ContainerID: "container_id"},
			},
			expectedContainerID: "container_id",
			setup:               func() {},
		},
		{
			name: "with ProcessID",
			originInfo: origindetection.OriginInfo{
				LocalData: origindetection.LocalData{ProcessID: 123},
			},
			expectedContainerID: "container_id",
			setup: func() {
				mockCollector := collectormock.MetaCollector{CIDFromPID: map[int]string{123: "container_id"}}
				mockMetricsProvider.RegisterMetaCollector(&mockCollector)
			},
		},
		{
			name: "with Inode",
			originInfo: origindetection.OriginInfo{
				LocalData: origindetection.LocalData{Inode: 123},
			},
			expectedContainerID: "container_id",
			setup: func() {
				mockCollector := collectormock.MetaCollector{CIDFromInode: map[uint64]string{123: "container_id"}}
				mockMetricsProvider.RegisterMetaCollector(&mockCollector)
			},
		},
		{
			name: "with External Data",
			originInfo: origindetection.OriginInfo{
				ExternalData: origindetection.ExternalData{
					ContainerName: "container_name",
					PodUID:        "pod_uid",
				},
			},
			expectedContainerID: "container_id",
			setup: func() {
				mockCollector := collectormock.MetaCollector{CIDFromPodUIDContName: map[string]string{"pod_uid/container_name": "container_id"}}
				mockMetricsProvider.RegisterMetaCollector(&mockCollector)
			},
		},
		{
			name: "with External Data and Init Container",
			originInfo: origindetection.OriginInfo{
				ExternalData: origindetection.ExternalData{
					Init:          true,
					ContainerName: "container_name",
					PodUID:        "pod_uid",
				},
			},
			expectedContainerID: "container_id",
			setup: func() {
				mockCollector := collectormock.MetaCollector{CIDFromPodUIDContName: map[string]string{"i-pod_uid/container_name": "container_id"}}
				mockMetricsProvider.RegisterMetaCollector(&mockCollector)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			containerID, err := fakeTagger.GenerateContainerIDFromOriginInfo(tt.originInfo)
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedContainerID, containerID)
			}
		})
	}
}

func TestGenerateContainerIDFromExternalData(t *testing.T) {
	store := fxutil.Test[workloadmeta.Component](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	logComponent := logmock.New(t)
	cfg := configmock.New(t)
	tagger, taggerErr := newLocalTagger(cfg, store, logComponent, telemetryComponent, nil)
	assert.NoError(t, taggerErr)

	for _, tt := range []struct {
		name         string
		externalData origindetection.ExternalData
		expected     string
		cidProvider  *fakeCIDProvider
	}{
		{
			name:         "empty",
			externalData: origindetection.ExternalData{},
			expected:     "",
			cidProvider:  &fakeCIDProvider{},
		},
		{
			name: "found container",
			externalData: origindetection.ExternalData{
				Init:          false,
				ContainerName: "containerName",
				PodUID:        "podUID",
			},
			expected: "containerID",
			cidProvider: &fakeCIDProvider{
				entries: map[string]string{
					"podUID/containerName": "containerID",
				},
				initEntries: map[string]string{},
			},
		},
		{
			name: "found init container",
			externalData: origindetection.ExternalData{
				Init:          true,
				ContainerName: "initContainerName",
				PodUID:        "podUID",
			},
			expected: "initContainerID",
			cidProvider: &fakeCIDProvider{
				entries: map[string]string{},
				initEntries: map[string]string{
					"podUID/initContainerName": "initContainerID",
				},
			},
		},
		{
			name: "container not found",
			externalData: origindetection.ExternalData{
				Init:          true,
				ContainerName: "containerName",
				PodUID:        "podUID",
			},
			expected: "",
			cidProvider: &fakeCIDProvider{
				entries: map[string]string{},
				initEntries: map[string]string{
					"podUID/initContainerName": "initContainerID",
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			containerID, err := tagger.generateContainerIDFromExternalData(tt.externalData, tt.cidProvider)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, containerID)
		})
	}
}

func TestGenerateContainerIDFromInode(t *testing.T) {
	store := fxutil.Test[workloadmeta.Component](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	logComponent := logmock.New(t)
	cfg := configmock.New(t)
	tagger, taggerErr := newLocalTagger(cfg, store, logComponent, telemetryComponent, nil)
	assert.NoError(t, taggerErr)

	// Create mock metrics provider
	mockProvider := collectormock.NewMetricsProvider()
	mockProvider.RegisterMetaCollector(&collectormock.MetaCollector{
		CIDFromInode: map[uint64]string{
			uint64(1234): "abcdef",
		},
	})

	for _, tt := range []struct {
		name          string
		localData     origindetection.LocalData
		expected      string
		inodeProvider *collectormock.MetricsProvider
	}{
		{
			name:          "empty",
			localData:     origindetection.LocalData{},
			expected:      "",
			inodeProvider: mockProvider,
		},
		{
			name: "found container",
			localData: origindetection.LocalData{
				Inode: 1234,
			},
			expected:      "abcdef",
			inodeProvider: mockProvider,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			containerID, err := tagger.generateContainerIDFromInode(tt.localData, mockProvider.GetMetaCollector())
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, containerID)
		})
	}
}

func TestDefaultCardinality(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		wantChecksCardinality types.TagCardinality
		setup                 func(cfg config.Component)
	}{
		{
			name:                  "successful parse config values, use config",
			wantChecksCardinality: types.HighCardinality,
			setup: func(cfg config.Component) {
				cfg.SetInTest("checks_tag_cardinality", types.HighCardinalityString)
			},
		},
		{
			name:                  "fail parse config values, use default",
			wantChecksCardinality: types.LowCardinality,
			setup: func(cfg config.Component) {
				cfg.SetInTest("checks_tag_cardinality", "foo")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			tt.setup(cfg)

			logComponent := logmock.New(t)
			wmeta := fxutil.Test[workloadmeta.Component](t,
				fx.Provide(func() log.Component { return logComponent }),
				fx.Provide(func() config.Component { return cfg }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)

			tagger, err := newLocalTagger(cfg, wmeta, logComponent, noopTelemetry.GetCompatComponent(), nil)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantChecksCardinality, tagger.datadogConfig.checksCardinality)
		})
	}
}

func TestTaggerCardinality(t *testing.T) {
	tests := []struct {
		name        string
		cardinality string
		want        types.TagCardinality
	}{
		{
			name:        "high",
			cardinality: "high",
			want:        types.HighCardinality,
		},
		{
			name:        "orchestrator",
			cardinality: "orchestrator",
			want:        types.OrchestratorCardinality,
		},
		{
			name:        "orch",
			cardinality: "orch",
			want:        types.OrchestratorCardinality,
		},
		{
			name:        "low",
			cardinality: "low",
			want:        types.LowCardinality,
		},
		{
			name:        "empty",
			cardinality: "",
			want:        types.LowCardinality,
		},
		{
			name:        "unknown",
			cardinality: "foo",
			want:        types.LowCardinality,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := logmock.New(t)
			assert.Equal(t, tt.want, taggerCardinality(tt.cardinality, types.LowCardinality, l))
		})
	}
}

func TestGlobalTags(t *testing.T) {
	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp

	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "bar"), "fooSource", []string{"container-low"}, []string{"container-orch"}, []string{"container-high"}, nil)
	fakeTagger.SetGlobalTags([]string{"global-low"}, []string{"global-orch"}, []string{"global-high"}, nil)

	globalTags, err := fakeTagger.GlobalTags(types.OrchestratorCardinality)
	assert.Nil(t, err)
	assert.Equal(t, []string{"global-low", "global-orch"}, globalTags)

	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{ContainerIDFromSocket: "container_id://bar", Cardinality: "orchestrator"})
	assert.Equal(t, []string{"container-low", "container-orch", "global-low", "global-orch"}, tb.Get())
}

func TestAgentTags(t *testing.T) {
	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp

	agentContainerID, podUID := "agentContainerID", "podUID"
	mockMetricsProvider := collectormock.NewMetricsProvider()
	cleanUp := setupFakeMetricsProvider(mockMetricsProvider)
	defer cleanUp()

	fakeTagger.SetTags(types.NewEntityID(types.KubernetesPodUID, podUID), "fooSource", []string{"pod-low"}, []string{"pod-orch"}, nil, nil)
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, agentContainerID), "fooSource", []string{"container-low"}, []string{"container-orch"}, nil, nil)

	// Expect metrics provider to return an empty container ID so no tags can be found
	containerMetaCollector := collectormock.MetaCollector{ContainerID: ""}
	mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector)
	tagList, err := fakeTagger.AgentTags(types.OrchestratorCardinality)
	assert.Nil(t, err)
	assert.Nil(t, tagList)

	// Expect metrics provider to return the agent container ID so tags can be found
	containerMetaCollector = collectormock.MetaCollector{ContainerID: agentContainerID}
	mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector)
	tagList, err = fakeTagger.AgentTags(types.OrchestratorCardinality)
	assert.NoError(t, err)
	assert.Equal(t, []string{"container-low", "container-orch"}, tagList)
}

func TestEnrichTagsOptOut(t *testing.T) {
	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp

	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "bar"), "fooSource", []string{"container-low"}, []string{"container-orch"}, nil, nil)

	tb := tagset.NewHashingTagsAccumulator()
	// Test with none cardinality
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{
		ContainerIDFromSocket: "container_id://bar",
		LocalData: origindetection.LocalData{
			ContainerID: "container-id",
		},
		Cardinality:   "none",
		ProductOrigin: origindetection.ProductOriginDogStatsD,
	})
	assert.Equal(t, []string{}, tb.Get())

	// Test without none cardinality
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{
		ContainerIDFromSocket: "container_id://bar",
		LocalData: origindetection.LocalData{
			ContainerID: "container-id",
			PodUID:      "pod-uid",
		},
		Cardinality:   "low",
		ProductOrigin: origindetection.ProductOriginDogStatsD,
	})
	assert.Equal(t, []string{"container-low"}, tb.Get())
}

func TestEnrichTagsOrchestrator(t *testing.T) {
	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp

	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "bar"), "fooSource", []string{"container-low"}, []string{"container-orch"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{ContainerIDFromSocket: "container_id://bar", Cardinality: "orchestrator"})
	assert.Equal(t, []string{"container-low", "container-orch"}, tb.Get())
}

func TestEnrichTags(t *testing.T) {
	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: noopTelemetry.GetCompatComponent(),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp

	containerName, initContainerName, containerID, initContainerID, podUID := "container-name", "init-container-name", "container-id", "init-container-id", "pod-uid"

	// Fill fake tagger with entities

	// Note: in the real tagger, the pod tags are tied to the container tags.
	// However, this is not the case in the fake tagger.
	// Hence, we should only expect the pod defined tags if tag enrichment directly
	// queries for the pod entity because higher granular container tags failed to accumulate.

	fakeTagger.SetTags(types.NewEntityID(types.KubernetesPodUID, podUID), "host", []string{"pod-low"}, []string{"pod-orch"}, []string{"pod-high"}, []string{"pod-std"})
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, containerID), "host", []string{"container-low"}, []string{"container-orch"}, []string{"container-high"}, []string{"container-std"})
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, initContainerID), "host", []string{"init-container-low"}, []string{"init-container-orch"}, []string{"init-container-high"}, []string{"init-container-std"})

	// Local data tests
	for _, tt := range []struct {
		name         string
		originInfo   taggertypes.OriginInfo
		expectedTags []string
	}{
		{
			name:         "no origin",
			originInfo:   taggertypes.OriginInfo{},
			expectedTags: []string{},
		},
		{
			name: "with local data (containerID) and low cardinality",
			originInfo: taggertypes.OriginInfo{
				LocalData: origindetection.LocalData{
					ContainerID: containerID,
				},
				Cardinality: "low",
			},
			expectedTags: []string{"container-low"},
		},
		{
			name: "with local data (containerID) and high cardinality",
			originInfo: taggertypes.OriginInfo{
				LocalData: origindetection.LocalData{
					ContainerID: containerID,
				},
				Cardinality: "high",
			}, expectedTags: []string{"container-low", "container-orch", "container-high"},
		},
		{
			name: "with local data (podUID) and low cardinality",
			originInfo: taggertypes.OriginInfo{
				LocalData: origindetection.LocalData{
					PodUID: podUID,
				},
				Cardinality: "low",
			},
			expectedTags: []string{"pod-low"},
		},
		{
			name: "with local data (podUID) and high cardinality",
			originInfo: taggertypes.OriginInfo{
				LocalData: origindetection.LocalData{
					PodUID: podUID,
				},
				Cardinality: "high",
			},
			expectedTags: []string{"pod-low", "pod-orch", "pod-high"},
		},
		{
			name: "with local data (podUID, containerIDFromSocket) and high cardinality, APM origin",
			originInfo: taggertypes.OriginInfo{
				ContainerIDFromSocket: "container_id://" + containerID,
				LocalData: origindetection.LocalData{
					PodUID: podUID,
				},
				Cardinality:   "high",
				ProductOrigin: origindetection.ProductOriginAPM},
			expectedTags: []string{"container-low", "container-orch", "container-high"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tb := tagset.NewHashingTagsAccumulator()
			fakeTagger.EnrichTags(tb, tt.originInfo)
			assert.Equal(t, tt.expectedTags, tb.Get())
		})
	}

	// External data tests

	// Overriding the GetProvider function to use the mock metrics provider
	mockMetricsProvider := collectormock.NewMetricsProvider()
	initContainerMetaCollector := collectormock.MetaCollector{ContainerID: initContainerID, CIDFromPodUIDContName: map[string]string{fmt.Sprint("i-", podUID, "/", initContainerName): initContainerID}}
	containerMetaCollector := collectormock.MetaCollector{ContainerID: containerID, CIDFromPodUIDContName: map[string]string{fmt.Sprint(podUID, "/", containerName): containerID}}
	cleanUp := setupFakeMetricsProvider(mockMetricsProvider)
	defer cleanUp()

	for _, tt := range []struct {
		name         string
		originInfo   taggertypes.OriginInfo
		expectedTags []string
		setup        func() // register the proper meta collector for the test
	}{
		{
			name: "with external data (containerName) and high cardinality",
			originInfo: taggertypes.OriginInfo{
				ProductOrigin: origindetection.ProductOriginAPM,
				ExternalData: origindetection.ExternalData{
					Init:          false,
					ContainerName: containerName,
				},
				Cardinality: "high",
			},
			expectedTags: []string{},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name: "with external data (containerName, podUID) and low cardinality",
			originInfo: taggertypes.OriginInfo{
				ProductOrigin: origindetection.ProductOriginAPM,
				ExternalData: origindetection.ExternalData{
					Init:          false,
					ContainerName: containerName,
					PodUID:        podUID,
				},
				Cardinality: "low",
			},
			expectedTags: []string{"container-low"},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name: "with external data (podUID) and high cardinality",
			originInfo: taggertypes.OriginInfo{
				ProductOrigin: origindetection.ProductOriginAPM,
				ExternalData: origindetection.ExternalData{
					Init:   false,
					PodUID: podUID,
				},
				Cardinality: "high",
			},
			expectedTags: []string{"pod-low", "pod-orch", "pod-high"},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name: "with external data (containerName, podUID) and high cardinality",
			originInfo: taggertypes.OriginInfo{
				ProductOrigin: origindetection.ProductOriginAPM,
				ExternalData: origindetection.ExternalData{
					Init:          false,
					ContainerName: containerName,
					PodUID:        podUID,
				},
				Cardinality: "high",
			},
			expectedTags: []string{"container-low", "container-orch", "container-high"},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name: "with external data (containerName, podUID, initContainer) and low cardinality",
			originInfo: taggertypes.OriginInfo{
				ProductOrigin: origindetection.ProductOriginAPM,
				ExternalData: origindetection.ExternalData{
					Init:          true,
					ContainerName: initContainerName,
					PodUID:        podUID,
				},
				Cardinality: "low",
			},
			expectedTags: []string{"init-container-low"},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&initContainerMetaCollector) },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			tb := tagset.NewHashingTagsAccumulator()
			fakeTagger.EnrichTags(tb, tt.originInfo)
			assert.Equal(t, tt.expectedTags, tb.Get())
		})
	}
}

func TestEnrichTagsContainerIDMismatch(t *testing.T) {
	mockReq := MockRequires{
		Config:    configmock.New(t),
		Log:       logmock.New(t),
		Telemetry: fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule()),
	}
	mockReq.WorkloadMeta = fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return mockReq.Config }),
		fx.Provide(func() log.Component { return mockReq.Log }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	fakeTagger := NewMock(mockReq).Comp

	localContainerID, externalContainerID, podUID := "local-container-id", "external-container-id", "pod-uid"
	containerName := "container-name"

	fakeTagger.SetTags(types.NewEntityID(types.KubernetesPodUID, podUID), "kubelet", []string{"directly-from-pod-low"}, nil, nil, nil)
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, localContainerID), "kubelet", []string{"local-container-low"}, nil, nil, nil)
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, externalContainerID), "kubelet", []string{"external-container-low"}, nil, nil, nil)

	// Set up fake metrics provider that returns a different container ID than the local data
	mockMetricsProvider := collectormock.NewMetricsProvider()
	externalContainerMetaCollector := collectormock.MetaCollector{
		ContainerID:           externalContainerID,
		CIDFromPodUIDContName: map[string]string{fmt.Sprint(podUID, "/", containerName): externalContainerID},
	}
	mockMetricsProvider.RegisterMetaCollector(&externalContainerMetaCollector)
	cleanUp := setupFakeMetricsProvider(mockMetricsProvider)
	defer cleanUp()

	t.Run("container ID mismatch - external data ignored", func(t *testing.T) {
		tb := tagset.NewHashingTagsAccumulator()
		originInfo := taggertypes.OriginInfo{
			ProductOrigin: origindetection.ProductOriginAPM,
			LocalData: origindetection.LocalData{
				ContainerID: localContainerID, // Local data has one container ID
				PodUID:      podUID,
			},
			ExternalData: origindetection.ExternalData{
				Init:          false,
				ContainerName: containerName, // External data resolves to different container ID
				PodUID:        podUID,
			},
			Cardinality: "high",
		}
		fakeTagger.EnrichTags(tb, originInfo)
		actualTags := tb.Get()

		assert.Contains(t, actualTags, "local-container-low")
		assert.NotContains(t, actualTags, "directly-from-pod-low")
		assert.NotContains(t, actualTags, "external-container-low")
	})
}
