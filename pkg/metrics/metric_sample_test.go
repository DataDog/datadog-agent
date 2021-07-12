// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util"

	"github.com/stretchr/testify/assert"
)

func TestMetricSampleCopy(t *testing.T) {
	src := &MetricSample{}
	src.Host = "foo"
	src.Mtype = HistogramType
	src.Name = "metric.name"
	src.RawValue = "0.1"
	src.SampleRate = 1
	src.Tags = []string{"a:b", "c:d"}
	src.Timestamp = 1234
	src.Value = 0.1
	dst := src.Copy()

	assert.False(t, src == dst)
	assert.True(t, reflect.DeepEqual(&src, &dst))
}

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
			want:        tagger.DogstatsdCardinality,
		},
		{
			name:        "unknown",
			cardinality: "foo",
			want:        tagger.DogstatsdCardinality,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, taggerCardinality(tt.cardinality))
		})
	}
}

func TestEnrichTagsOrchestrator(t *testing.T) {
	oldTagger := tagger.GetDefaultTagger()
	defer tagger.SetDefaultTagger(oldTagger)

	fakeTagger := local.NewFakeTagger()
	tagger.SetDefaultTagger(fakeTagger)
	fakeTagger.SetTags("foo", "fooSource", []string{"lowTag"}, []string{"orchTag"}, nil, nil)

	tb := util.NewTagsBuilder()
	EnrichTags(tb, "foo", "", "orchestrator")
	assert.Equal(t, []string{"lowTag", "orchTag"}, tb.Get())
}
