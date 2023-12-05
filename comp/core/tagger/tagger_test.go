// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

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
	taggerClient := fxutil.Test[Component](t,
		core.MockBundle,
		fx.Supply(NewFakeTaggerParams()),
		fx.Provide(func() context.Context { return context.TODO() }),
		Module,
	)
	fakeTagger := GetDefaultTagger(taggerClient).(*local.FakeTagger)
	defer ResetGlobalTaggerClient(nil)
	fakeTagger.SetTags("foo", "fooSource", []string{"lowTag"}, []string{"orchTag"}, nil, nil)
	tb := tagset.NewHashingTagsAccumulator()
	EnrichTags(tb, "foo", "", "orchestrator")
	assert.Equal(t, []string{"lowTag", "orchTag"}, tb.Get())
}
