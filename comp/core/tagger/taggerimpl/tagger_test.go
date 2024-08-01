// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// TODO Improve test coverage with dogstatsd/enrich tests once Origin Detection is refactored.

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

func TestEnrichTags(t *testing.T) {
	// Create fake tagger
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())
	defer fakeTagger.ResetTagger()

	// Fill fake tagger with entities
	fakeTagger.SetTags(kubelet.KubePodTaggerEntityPrefix+"pod", "host", []string{"pod-low"}, []string{"pod-orch"}, []string{"pod-high"}, []string{"pod-std"})
	fakeTagger.SetTags(containers.BuildTaggerEntityName("container"), kubelet.KubePodTaggerEntityPrefix+"pod", []string{"container-low"}, []string{"container-orch"}, []string{"container-high"}, []string{"container-std"})

	for _, tt := range []struct {
		name         string
		originInfo   taggertypes.OriginInfo
		cidProvider  *fakeCIDProvider
		expectedTags []string
	}{
		{
			name:         "no origin",
			originInfo:   taggertypes.OriginInfo{},
			expectedTags: []string{},
			cidProvider:  &fakeCIDProvider{},
		},
		{
			name:         "with local data (containerID) and low cardinality",
			originInfo:   taggertypes.OriginInfo{FromMsg: "container", Cardinality: "low"},
			expectedTags: []string{"container-low"},
			cidProvider:  &fakeCIDProvider{},
		},
		{
			name:         "with local data (containerID) and high cardinality",
			originInfo:   taggertypes.OriginInfo{FromMsg: "container", Cardinality: "high"},
			expectedTags: []string{"container-low", "container-orch", "container-high"},
			cidProvider:  &fakeCIDProvider{},
		},
		{
			name:         "with local data (podUID) and low cardinality",
			originInfo:   taggertypes.OriginInfo{FromTag: "pod", Cardinality: "low"},
			expectedTags: []string{"pod-low"},
			cidProvider:  &fakeCIDProvider{},
		},
		{
			name:         "with local data (podUID) and high cardinality",
			originInfo:   taggertypes.OriginInfo{FromTag: "pod", Cardinality: "high"},
			expectedTags: []string{"pod-low", "pod-orch", "pod-high"},
			cidProvider:  &fakeCIDProvider{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tb := tagset.NewHashingTagsAccumulator()
			fakeTagger.EnrichTags(tb, tt.originInfo)
			assert.Equal(t, tt.expectedTags, tb.Get())
		})
	}
}

func TestEnrichTagsOrchestrator(t *testing.T) {
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())
	defer fakeTagger.ResetTagger()
	fakeTagger.SetTags("foo", "fooSource", []string{"lowTag"}, []string{"orchTag"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "foo", Cardinality: "orchestrator"})
	assert.Equal(t, []string{"lowTag", "orchTag"}, tb.Get())
}

func TestEnrichTagsOptOut(t *testing.T) {
	fakeTagger := fxutil.Test[tagger.Mock](t, MockModule())
	defer fakeTagger.ResetTagger()
	cfg := configmock.New(t)
	cfg.SetWithoutSource("dogstatsd_origin_optout_enabled", true)
	fakeTagger.SetTags("foo", "fooSource", []string{"lowTag"}, []string{"orchTag"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "originID", FromTag: "pod-uid", FromMsg: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
	assert.Equal(t, []string{}, tb.Get())
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "originID", FromMsg: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
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
			want:        tagger.DogstatsdCardinality(),
		},
		{
			name:        "unknown",
			cardinality: "foo",
			want:        tagger.DogstatsdCardinality(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := logmock.New(t)
			assert.Equal(t, tt.want, taggerCardinality(tt.cardinality, tagger.DogstatsdCardinality(), l))
		})
	}
}
