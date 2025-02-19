// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	collectormock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
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
	logComponent := logmock.New(t)
	telemetryStore := taggerTelemetry.NewStore(tel)
	cfg := configmock.New(t)
	tagger, err := newLocalTagger(cfg, store, logComponent, telemetryStore)
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
	logComponent := logmock.New(t)
	telemetryStore := taggerTelemetry.NewStore(tel)
	cfg := configmock.New(t)
	tagger, err := newLocalTagger(cfg, store, logComponent, telemetryStore)
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

func TestGenerateContainerIDFromOriginInfo(t *testing.T) {
	store := fxutil.Test[workloadmeta.Component](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	tel := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	logComponent := logmock.New(t)
	telemetryStore := taggerTelemetry.NewStore(tel)
	cfg := configmock.New(t)
	tagger, taggerErr := newLocalTagger(cfg, store, logComponent, telemetryStore)
	assert.NoError(t, taggerErr)
	localTagger := tagger.(*localTagger)

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
			containerID, err := localTagger.GenerateContainerIDFromOriginInfo(tt.originInfo)
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
