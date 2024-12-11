// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestAccumulateTagsFor(t *testing.T) {
	entityID := types.NewEntityID("", "entity_name")

	store := fxutil.Test[workloadmeta.Component](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	tel := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := taggerTelemetry.NewStore(tel)
	cfg := configmock.New(t)
	tagger, err := newLocalTagger(cfg, store, telemetryStore)
	assert.NoError(t, err)
	localTagger := tagger.(*localTagger)
	localTagger.Start(context.Background())
	defer tagger.Stop()

	localTagger.tagStore.ProcessTagInfo([]*types.TagInfo{
		{
			EntityID:     entityID,
			Source:       "stream",
			LowCardTags:  []string{"low1"},
			HighCardTags: []string{"high"},
		},
		{
			EntityID:    entityID,
			Source:      "pull",
			LowCardTags: []string{"low2"},
		},
	})

	tb := tagset.NewHashlessTagsAccumulator()
	err = localTagger.AccumulateTagsFor(entityID, types.HighCardinality, tb)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"high", "low1", "low2"}, tb.Get())
}

func TestTag(t *testing.T) {
	entityID := types.NewEntityID(types.ContainerID, "123")

	store := fxutil.Test[workloadmeta.Component](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	tel := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := taggerTelemetry.NewStore(tel)
	cfg := configmock.New(t)
	tagger, err := newLocalTagger(cfg, store, telemetryStore)
	assert.NoError(t, err)
	localTagger := tagger.(*localTagger)

	localTagger.tagStore.ProcessTagInfo([]*types.TagInfo{
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

	lowCardTags, err := localTagger.Tag(entityID, types.LowCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2"}, lowCardTags)

	orchestratorCardTags, err := localTagger.Tag(entityID, types.OrchestratorCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2", "orchestrator1", "orchestrator2"}, orchestratorCardTags)

	highCardTags, err := localTagger.Tag(entityID, types.HighCardinality)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"low1", "low2", "orchestrator1", "orchestrator2", "high1", "high2"}, highCardTags)
}
