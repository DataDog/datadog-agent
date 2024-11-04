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

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	collectormock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
	metrics.GetProvider = func(_ optional.Option[workloadmeta.Component]) metrics.Provider {
		return mockMetricsProvider
	}
	return func() { metrics.GetProvider = originalMetricsProvider }
}

func TestEnrichTags(t *testing.T) {
	// Create fake tagger
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())
	containerName, initContainerName, containerID, initContainerID, podUID := "container-name", "init-container-name", "container-id", "init-container-id", "pod-uid"

	// Fill fake tagger with entities
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
			name:         "with local data (containerID) and low cardinality",
			originInfo:   taggertypes.OriginInfo{ContainerID: containerID, Cardinality: "low"},
			expectedTags: []string{"container-low"},
		},
		{
			name:         "with local data (containerID) and high cardinality",
			originInfo:   taggertypes.OriginInfo{ContainerID: containerID, Cardinality: "high"},
			expectedTags: []string{"container-low", "container-orch", "container-high"},
		},
		{
			name:         "with local data (podUID) and low cardinality",
			originInfo:   taggertypes.OriginInfo{PodUID: podUID, Cardinality: "low"},
			expectedTags: []string{"pod-low"},
		},
		{
			name:         "with local data (podUID) and high cardinality",
			originInfo:   taggertypes.OriginInfo{PodUID: podUID, Cardinality: "high"},
			expectedTags: []string{"pod-low", "pod-orch", "pod-high"},
		},
		{
			name:         "with local data (podUID, containerIDFromSocket) and high cardinality, APM origin",
			originInfo:   taggertypes.OriginInfo{PodUID: podUID, Cardinality: "high", ContainerIDFromSocket: fmt.Sprintf("container_id://%s", containerID), ProductOrigin: taggertypes.ProductOriginAPM},
			expectedTags: []string{"container-low", "container-orch", "container-high", "pod-low", "pod-orch", "pod-high"},
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
			name:         "with external data (containerName) and high cardinality",
			originInfo:   taggertypes.OriginInfo{ProductOrigin: taggertypes.ProductOriginAPM, ExternalData: fmt.Sprintf("cn-%s,it-false", containerName), Cardinality: "high"},
			expectedTags: []string{},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name:         "with external data (containerName, podUID) and low cardinality",
			originInfo:   taggertypes.OriginInfo{ProductOrigin: taggertypes.ProductOriginAPM, ExternalData: fmt.Sprintf("it-invalid,cn-%s,pu-%s", containerName, podUID), Cardinality: "low"},
			expectedTags: []string{"pod-low", "container-low"},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name:         "with external data (podUID) and high cardinality",
			originInfo:   taggertypes.OriginInfo{ProductOrigin: taggertypes.ProductOriginAPM, ExternalData: fmt.Sprintf("pu-%s,it-false", podUID), Cardinality: "high"},
			expectedTags: []string{"pod-low", "pod-orch", "pod-high"},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name:         "with external data (containerName, podUID) and high cardinality",
			originInfo:   taggertypes.OriginInfo{ProductOrigin: taggertypes.ProductOriginAPM, ExternalData: fmt.Sprintf("pu-%s,it-false,cn-%s", podUID, containerName), Cardinality: "high"},
			expectedTags: []string{"pod-low", "pod-orch", "pod-high", "container-low", "container-orch", "container-high"},
			setup:        func() { mockMetricsProvider.RegisterMetaCollector(&containerMetaCollector) },
		},
		{
			name:         "with external data (containerName, podUID, initContainer) and low cardinality",
			originInfo:   taggertypes.OriginInfo{ProductOrigin: taggertypes.ProductOriginAPM, ExternalData: fmt.Sprintf("pu-%s,cn-%s,it-true", podUID, initContainerName), Cardinality: "low"},
			expectedTags: []string{"pod-low", "init-container-low"},
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

func TestEnrichTagsOrchestrator(t *testing.T) {
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "bar"), "fooSource", []string{"container-low"}, []string{"container-orch"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{ContainerIDFromSocket: "container_id://bar", Cardinality: "orchestrator"})
	assert.Equal(t, []string{"container-low", "container-orch"}, tb.Get())
}

func TestEnrichTagsOptOut(t *testing.T) {
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())

	cfg := configmock.New(t)
	cfg.SetWithoutSource("dogstatsd_origin_optout_enabled", true)
	fakeTagger.SetTags(types.NewEntityID(types.EntityIDPrefix("foo"), "bar"), "fooSource", []string{"container-low"}, []string{"container-orch"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{ContainerIDFromSocket: "foo://bar", PodUID: "pod-uid", ContainerID: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
	assert.Equal(t, []string{}, tb.Get())
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{ContainerIDFromSocket: "foo://bar", ContainerID: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
	assert.Equal(t, []string{}, tb.Get())
}

func TestGenerateContainerIDFromExternalData(t *testing.T) {
	for _, tt := range []struct {
		name         string
		externalData externalData
		expected     string
		cidProvider  *fakeCIDProvider
	}{
		{
			name:         "empty",
			externalData: externalData{},
			expected:     "",
			cidProvider:  &fakeCIDProvider{},
		},
		{
			name: "found container",
			externalData: externalData{
				init:          false,
				containerName: "containerName",
				podUID:        "podUID",
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
			externalData: externalData{
				init:          true,
				containerName: "initContainerName",
				podUID:        "podUID",
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
			externalData: externalData{
				init:          true,
				containerName: "containerName",
				podUID:        "podUID",
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
			fakeTagger := TaggerClient{}
			containerID, err := fakeTagger.generateContainerIDFromExternalData(tt.externalData, tt.cidProvider)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, containerID)
		})
	}
}

func TestAgentTags(t *testing.T) {
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())

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

func TestGlobalTags(t *testing.T) {
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "bar"), "fooSource", []string{"container-low"}, []string{"container-orch"}, []string{"container-high"}, nil)
	fakeTagger.SetGlobalTags([]string{"global-low"}, []string{"global-orch"}, []string{"global-high"}, nil)

	globalTags, err := fakeTagger.GlobalTags(types.OrchestratorCardinality)
	assert.Nil(t, err)
	assert.Equal(t, []string{"global-low", "global-orch"}, globalTags)

	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{ContainerIDFromSocket: "container_id://bar", Cardinality: "orchestrator"})
	assert.Equal(t, []string{"container-low", "container-orch", "global-low", "global-orch"}, tb.Get())
}

func TestTaggerCardinality(t *testing.T) {
	fakeTagger := TaggerClient{}
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
			want:        fakeTagger.DogstatsdCardinality(),
		},
		{
			name:        "unknown",
			cardinality: "foo",
			want:        fakeTagger.DogstatsdCardinality(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := logmock.New(t)
			assert.Equal(t, tt.want, taggerCardinality(tt.cardinality, fakeTagger.DogstatsdCardinality(), l))
		})
	}
}

func TestDefaultCardinality(t *testing.T) {
	cfg := configmock.New(t)
	for _, tt := range []struct {
		name                     string
		wantChecksCardinality    types.TagCardinality
		wantDogstatsdCardinality types.TagCardinality
		setup                    func()
	}{
		{
			name:                     "successful parse config values, use config",
			wantChecksCardinality:    types.HighCardinality,
			wantDogstatsdCardinality: types.OrchestratorCardinality,
			setup: func() {
				cfg.SetWithoutSource("checks_tag_cardinality", types.HighCardinalityString)
				cfg.SetWithoutSource("dogstatsd_tag_cardinality", types.OrchestratorCardinalityString)
			},
		},
		{
			name:                     "fail parse config values, use default",
			wantChecksCardinality:    types.LowCardinality,
			wantDogstatsdCardinality: types.LowCardinality,
			setup: func() {
				cfg.SetWithoutSource("checks_tag_cardinality", "foo")
				cfg.SetWithoutSource("dogstatsd_tag_cardinality", "foo")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())
			assert.Equal(t, tt.wantDogstatsdCardinality, fakeTagger.DogstatsdCardinality())
			assert.Equal(t, tt.wantChecksCardinality, fakeTagger.ChecksCardinality())
		})
	}
}
