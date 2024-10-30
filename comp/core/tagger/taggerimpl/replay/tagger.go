// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package replay implements the Tagger replay.
package replay

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/empty"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tagger stores tags to entity as stored in a replay state.
type Tagger struct {
	store *tagstore.TagStore

	ctx    context.Context
	cancel context.CancelFunc

	telemetryTicker *time.Ticker
	telemetryStore  *telemetry.Store
	empty.Tagger
}

// NewTagger returns an allocated tagger. You still have to run Init()
// once the config package is ready.
func NewTagger(cfg config.Component, telemetryStore *telemetry.Store) *Tagger {
	return &Tagger{
		store:          tagstore.NewTagStore(cfg, telemetryStore),
		telemetryStore: telemetryStore,
	}
}

// Start starts the connection to the replay tagger and starts watching for
// events.
func (t *Tagger) Start(ctx context.Context) error {
	t.telemetryTicker = time.NewTicker(1 * time.Minute)

	t.ctx, t.cancel = context.WithCancel(ctx)

	return nil
}

// Stop closes the connection to the replay tagger and stops event collection.
func (t *Tagger) Stop() error {
	t.cancel()

	t.telemetryTicker.Stop()

	log.Info("replay tagger stopped successfully")

	return nil
}

// Tag returns tags for a given entity at the desired cardinality.
func (t *Tagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	tags := t.store.Lookup(entityID, cardinality)
	return tags, nil
}

// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
// If possible, avoid using this function, and use the Tag method instead.
// This function exists in order not to break backward compatibility with rtloader and python
// integrations using the tagger
func (t *Tagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := taggercommon.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// AccumulateTagsFor returns tags for a given entity at the desired cardinality.
func (t *Tagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
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
func (t *Tagger) Standard(entityID types.EntityID) ([]string, error) {
	tags, err := t.store.LookupStandard(entityID)
	if err != nil {
		return []string{}, err
	}

	return tags, nil
}

// List returns all the entities currently stored by the tagger.
func (t *Tagger) List() types.TaggerListResponse {
	return t.store.List()
}

// Subscribe does nothing in the replay tagger this tagger does not respond to events.
func (t *Tagger) Subscribe(_ string, _ *types.Filter) (types.Subscription, error) {
	// NOP
	return nil, fmt.Errorf("not implemented")
}

// ReplayTagger returns the replay tagger instance
func (t *Tagger) ReplayTagger() tagger.ReplayTagger {
	return t
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (t *Tagger) GetTaggerTelemetryStore() *telemetry.Store {
	return t.telemetryStore
}

// LoadState loads the state for the tagger from the supplied map.
func (t *Tagger) LoadState(state []types.Entity) {
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
func (t *Tagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return t.store.GetEntity(entityID)
}
