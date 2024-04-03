// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagger

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/config"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TODO Improve test coverage with dogstatsd/enrich tests once Origin Detection is refactored.

func Test_taggerCardinality(t *testing.T) {
	tests := []struct {
		name        string
		cardinality string
		want        collectors.TagCardinality
	}{
		{
			name:        "high",
			cardinality: "high",
			want:        collectors.HighCardinality,
		},
		{
			name:        "orchestrator",
			cardinality: "orchestrator",
			want:        collectors.OrchestratorCardinality,
		},
		{
			name:        "orch",
			cardinality: "orch",
			want:        collectors.OrchestratorCardinality,
		},
		{
			name:        "low",
			cardinality: "low",
			want:        collectors.LowCardinality,
		},
		{
			name:        "empty",
			cardinality: "",
			want:        DogstatsdCardinality,
		},
		{
			name:        "unknown",
			cardinality: "foo",
			want:        DogstatsdCardinality,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, taggerCardinality(tt.cardinality))
		})
	}
}

func TestEnrichTagsOrchestrator(t *testing.T) {
	fakeTagger := fxutil.Test[Mock](t, MockModule())
	defer fakeTagger.ResetTagger()
	fakeTagger.SetTags("foo", "fooSource", []string{"lowTag"}, []string{"orchTag"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "foo", Cardinality: "orchestrator"})
	assert.Equal(t, []string{"lowTag", "orchTag"}, tb.Get())
}

func TestEnrichTagsOptOut(t *testing.T) {
	fakeTagger := fxutil.Test[Mock](t, MockModule())
	defer fakeTagger.ResetTagger()
	cfg := config.Mock(t)
	cfg.SetWithoutSource("dogstatsd_origin_optout_enabled", true)
	fakeTagger.SetTags("foo", "fooSource", []string{"lowTag"}, []string{"orchTag"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "originID", FromTag: "pod-uid", FromMsg: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
	assert.Equal(t, []string{}, tb.Get())
	EnrichTags(tb, taggertypes.OriginInfo{FromUDS: "originID", FromMsg: "container-id", Cardinality: "none", ProductOrigin: taggertypes.ProductOriginDogStatsD})
	assert.Equal(t, []string{}, tb.Get())
}
