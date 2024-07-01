// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// TODO Improve test coverage with dogstatsd/enrich tests once Origin Detection is refactored.

func Test_taggerCardinality(t *testing.T) {
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
			assert.Equal(t, tt.want, taggerCardinality(tt.cardinality, tagger.DogstatsdCardinality()))
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
	cfg := config.Mock(t)
	cfg.SetWithoutSource("dogstatsd_origin_optout_enabled", true)
	fakeTagger.SetTags("foo", "fooSource", []string{"lowTag"}, []string{"orchTag"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "originID", FromTag: "pod-uid", FromMsg: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
	assert.Equal(t, []string{}, tb.Get())
	fakeTagger.EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "originID", FromMsg: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
	assert.Equal(t, []string{}, tb.Get())
}

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

func TestParseEntityID(t *testing.T) {
	for _, tt := range []struct {
		name        string
		entityID    string
		expected    string
		cidProvider *fakeCIDProvider
	}{
		{
			name:        "empty",
			entityID:    "",
			expected:    kubelet.KubePodTaggerEntityPrefix,
			cidProvider: &fakeCIDProvider{},
		},
		{
			name:        "pod uid",
			entityID:    "my-pod_uid",
			expected:    kubelet.KubePodTaggerEntityPrefix + "my-pod_uid",
			cidProvider: &fakeCIDProvider{},
		},
		{
			name:     "container + pod uid",
			entityID: "en-62381f4f-a19f-4f37-9413-90b738f92f83/appp",
			expected: containers.BuildTaggerEntityName("cid"),
			cidProvider: &fakeCIDProvider{
				entries: map[string]string{
					"62381f4f-a19f-4f37-9413-90b738f92f83/appp": "cid",
				},
			},
		},
		{
			name:     "init container + pod uid",
			entityID: "en-init.62381f4f-a19f-4f37-9413-90b738f92f83/appp",
			expected: containers.BuildTaggerEntityName("init-cid"),
			cidProvider: &fakeCIDProvider{
				initEntries: map[string]string{
					"62381f4f-a19f-4f37-9413-90b738f92f83/appp": "init-cid",
				},
			},
		},
		{
			name:        "not found",
			entityID:    "en-init.62381f4f-a19f-4f37-9413-90b738f92f83/init-my-cont_name",
			cidProvider: &fakeCIDProvider{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fakeCl := TaggerClient{}
			assert.Equal(t, tt.expected, fakeCl.parseEntityID(tt.entityID, tt.cidProvider))
		})
	}
}
