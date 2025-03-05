// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The Tagger is the central source of truth for client-side entity tagging.
// It subscribes to workloadmeta to get updates for all the entity kinds
// (containers, kubernetes pods, kubernetes nodes, etc.) and extracts the tags for each of them.
// Tags are then stored in memory (by the tagStore) and can be queried by the tagger.Tag()
// method.

// Package taggerimpl contains the implementation of the tagger component.
package taggerimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Requires defines the dependencies of the tagger component.
type Requires struct {
	compdef.In

	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Wmeta     workloadmeta.Component
	Telemetry coretelemetry.Component
}

// Provides contains the fields provided by the tagger constructor.
type Provides struct {
	compdef.Out

	Comp     tagger.Component
	Endpoint api.AgentEndpointProvider
}

// TaggerWrapper is a struct that contains two tagger component: capturetagger and the local tagger
// and implements the tagger interface
type TaggerWrapper struct {
	defaultTagger tagger.Component

	wmeta workloadmeta.Component

	log log.Component
}

// NewComponent returns a new tagger client
func NewComponent(req Requires) (Provides, error) {
	taggerClient, err := NewTaggerClient(req.Config, req.Wmeta, req.Log, req.Telemetry)

	if err != nil {
		return Provides{}, err
	}

	taggerClient.wmeta = req.Wmeta

	req.Log.Info("TaggerClient is created, defaultTagger type: ", reflect.TypeOf(taggerClient.defaultTagger))
	req.Lc.Append(compdef.Hook{OnStart: func(_ context.Context) error {
		// Main context passed to components, consistent with the one used in the workloadmeta component
		mainCtx, _ := common.GetMainCtxCancel()
		return taggerClient.Start(mainCtx)
	}})
	req.Lc.Append(compdef.Hook{OnStop: func(context.Context) error {
		return taggerClient.Stop()
	}})

	return Provides{
		Comp:     taggerClient,
		Endpoint: api.NewAgentEndpointProvider(taggerClient.writeList, "/tagger-list", "GET"),
	}, nil
}

// NewTaggerClient returns a new tagger client
func NewTaggerClient(cfg config.Component, wmeta workloadmeta.Component, log log.Component, telemetryComp coretelemetry.Component) (*TaggerWrapper, error) {
	var defaultTagger tagger.Component
	var err error
	defaultTagger, err = newLocalTagger(cfg, wmeta, log, telemetryComp, nil)

	if err != nil {
		return nil, err
	}

	wrapper := &TaggerWrapper{
		defaultTagger: defaultTagger,
		log:           log,
	}

	return wrapper, nil
}

func (t *TaggerWrapper) writeList(w http.ResponseWriter, _ *http.Request) {
	response := t.List()

	jsonTags, err := json.Marshal(response)
	if err != nil {
		httputils.SetJSONError(w, t.log.Errorf("Unable to marshal tagger list response: %s", err), 500)
		return
	}
	w.Write(jsonTags)
}

// Start calls defaultTagger.Start
func (t *TaggerWrapper) Start(ctx context.Context) error {
	return t.defaultTagger.Start(ctx)
}

// Stop calls defaultTagger.Stop
func (t *TaggerWrapper) Stop() error {
	return t.defaultTagger.Stop()
}

// GetTaggerTelemetryStore calls defaultTagger.GetTaggerTelemetryStore.
func (t *TaggerWrapper) GetTaggerTelemetryStore() *telemetry.Store {
	return t.defaultTagger.GetTaggerTelemetryStore()
}

// GetDefaultTagger returns the default Tagger in current instance
func (t *TaggerWrapper) GetDefaultTagger() tagger.Component {
	return t.defaultTagger
}

// GetEntity calls defaultTagger.GetEntity.
func (t *TaggerWrapper) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return t.defaultTagger.GetEntity(entityID)
}

// Tag calls defaultTagger.Tag.
func (t *TaggerWrapper) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	return t.defaultTagger.Tag(entityID, cardinality)
}

// LegacyTag calls defaultTagger.LegacyTag.
func (t *TaggerWrapper) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	return t.defaultTagger.LegacyTag(entity, cardinality)
}

// AccumulateTagsFor calls defaultTagger.AccumulateTagsFor.
func (t *TaggerWrapper) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	return t.defaultTagger.AccumulateTagsFor(entityID, cardinality, tb)
}

// GetEntityHash calls defaultTagger.GetEntityHash.
func (t *TaggerWrapper) GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string {
	return t.defaultTagger.GetEntityHash(entityID, cardinality)
}

// Standard calls defaultTagger.Standard.
func (t *TaggerWrapper) Standard(entityID types.EntityID) ([]string, error) {
	return t.defaultTagger.Standard(entityID)
}

// AgentTags calls defaultTagger.AgentTags.
func (t *TaggerWrapper) AgentTags(cardinality types.TagCardinality) ([]string, error) {
	return t.defaultTagger.AgentTags(cardinality)
}

// GlobalTags calls defaultTagger.GlobalTags.
func (t *TaggerWrapper) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return t.defaultTagger.GlobalTags(cardinality)
}

// List calls defaultTagger.List.
func (t *TaggerWrapper) List() types.TaggerListResponse {
	return t.defaultTagger.List()
}

// EnrichTags calls defaultTagger.EnrichTags.
func (t *TaggerWrapper) EnrichTags(tagAccumulator tagset.TagsAccumulator, originInfo taggertypes.OriginInfo) {
	t.defaultTagger.EnrichTags(tagAccumulator, originInfo)
}

// GenerateContainerIDFromOriginInfo calls defaultTagger.GenerateContainerIDFromOriginInfo.
func (t *TaggerWrapper) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error) {
	return t.defaultTagger.GenerateContainerIDFromOriginInfo(originInfo)
}

// ChecksCardinality calls defaultTagger.ChecksCardinality.
func (t *TaggerWrapper) ChecksCardinality() types.TagCardinality {
	return t.defaultTagger.ChecksCardinality()
}

// Subscribe calls defaultTagger.Subscribe.
func (t *TaggerWrapper) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return t.defaultTagger.Subscribe(subscriptionID, filter)
}
