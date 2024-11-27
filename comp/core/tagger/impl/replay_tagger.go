// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"context"
	"fmt"
	"time"

	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type replayTagger struct {
	store *tagstore.TagStore

	ctx    context.Context
	cancel context.CancelFunc

	telemetryTicker *time.Ticker
	telemetryStore  *telemetry.Store
}

func newReplayTagger(telemetryStore *telemetry.Store) tagger.ReplayTagger {
	return &replayTagger{
		store:          tagstore.NewTagStore(telemetryStore),
		telemetryStore: telemetryStore,
	}
}

// Start starts the connection to the replay tagger and starts watching for
// events.
func (t *replayTagger) Start(ctx context.Context) error {
	t.telemetryTicker = time.NewTicker(1 * time.Minute)

	t.ctx, t.cancel = context.WithCancel(ctx)

	return nil
}

// Stop closes the connection to the replay tagger and stops event collection.
func (t *replayTagger) Stop() error {
	t.cancel()

	t.telemetryTicker.Stop()

	log.Info("replay tagger stopped successfully")

	return nil
}

// Tag returns tags for a given entity at the desired cardinality.
func (t *replayTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	tags := t.store.Lookup(entityID, cardinality)
	return tags, nil
}

// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
// If possible, avoid using this function, and use the Tag method instead.
// This function exists in order not to break backward compatibility with rtloader and python
// integrations using the tagger
func (t *replayTagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := taggercommon.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// AccumulateTagsFor returns tags for a given entity at the desired cardinality.
func (t *replayTagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags := t.store.LookupHashed(entityID, cardinality)

	if tags.Len() == 0 {
		t.telemetryStore.QueriesByCardinality(cardinality).EmptyTags.Inc()
		return nil
	}

	t.telemetryStore.QueriesByCardinality(cardinality).Success.Inc()
	tb.AppendHashed(tags)

	return nil
}

// Standard returns the standard tags for a given entity.
func (t *replayTagger) Standard(entityID types.EntityID) ([]string, error) {
	tags, err := t.store.LookupStandard(entityID)
	if err != nil {
		return []string{}, err
	}

	return tags, nil
}

// List returns all the entities currently stored by the tagger.
func (t *replayTagger) List() types.TaggerListResponse {
	return t.store.List()
}

// Subscribe does nothing in the replay tagger this tagger does not respond to events.
func (t *replayTagger) Subscribe(_ string, _ *types.Filter) (types.Subscription, error) {
	// NOP
	return nil, fmt.Errorf("not implemented")
}

// ReplayTagger returns the replay tagger instance
func (t *replayTagger) ReplayTagger() tagger.ReplayTagger {
	return nil
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (t *replayTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return t.telemetryStore
}

// LoadState loads the state for the tagger from the supplied map.
func (t *replayTagger) LoadState(state []types.Entity) {
	if state == nil {
		return
	}

	for _, entity := range state {
		t.store.ProcessTagInfo([]*types.TagInfo{{
			Source:               "replay",
			EntityID:             entity.ID,
			HighCardTags:         entity.HighCardinalityTags,
			OrchestratorCardTags: entity.OrchestratorCardinalityTags,
			LowCardTags:          entity.LowCardinalityTags,
			StandardTags:         entity.StandardTags,
			ExpiryDate:           time.Time{},
		}})
	}

	log.Debugf("Loaded %v elements into tag store", len(state))
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *replayTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return t.store.GetEntity(entityID)
}

func (t *replayTagger) GetEntityHash(types.EntityID, types.TagCardinality) string {
	return ""
}

func (t *replayTagger) AgentTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func (t *replayTagger) GlobalTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func (t *replayTagger) SetNewCaptureTagger(tagger.Component) {}

func (t *replayTagger) ResetCaptureTagger() {}

func (t *replayTagger) EnrichTags(tagset.TagsAccumulator, taggertypes.OriginInfo) {}

func (t *replayTagger) ChecksCardinality() types.TagCardinality {
	return types.LowCardinality
}

func (t *replayTagger) DogstatsdCardinality() types.TagCardinality {
	return types.LowCardinality
}
