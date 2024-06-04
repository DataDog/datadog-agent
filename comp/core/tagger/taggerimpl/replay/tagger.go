// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package replay implements the Tagger replay.
package replay

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/proto"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/empty"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tagger stores tags to entity as stored in a replay state.
type Tagger struct {
	store *tagstore.TagStore

	ctx    context.Context
	cancel context.CancelFunc

	telemetryTicker *time.Ticker
	empty.Tagger
}

// NewTagger returns an allocated tagger. You still have to run Init()
// once the config package is ready.
func NewTagger() *Tagger {
	return &Tagger{
		// store: tagstore.NewTagStore(),
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
func (t *Tagger) Tag(entityID string, cardinality types.TagCardinality) ([]string, error) {
	tags := t.store.Lookup(entityID, cardinality)
	return tags, nil
}

// AccumulateTagsFor returns tags for a given entity at the desired cardinality.
func (t *Tagger) AccumulateTagsFor(entityID string, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags := t.store.LookupHashed(entityID, cardinality)

	if tags.Len() == 0 {
		// telemetry.QueriesByCardinality(cardinality).EmptyTags.Inc()
		return nil
	}

	// telemetry.QueriesByCardinality(cardinality).Success.Inc()
	tb.AppendHashed(tags)

	return nil
}

// Standard returns the standard tags for a given entity.
func (t *Tagger) Standard(entityID string) ([]string, error) {
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
func (t *Tagger) Subscribe(types.TagCardinality) chan []types.EntityEvent {
	// NOP
	return nil
}

// Unsubscribe does nothing in the replay tagger this tagger does not respond to events.
func (t *Tagger) Unsubscribe(chan []types.EntityEvent) {
	// NOP
}

// LoadState loads the state for the tagger from the supplied map.
func (t *Tagger) LoadState(state map[string]*pb.Entity) {
	if state == nil {
		return
	}

	// better stores these as the native type
	for id, entity := range state {
		entityID, err := proto.Pb2TaggerEntityID(entity.Id)
		if err != nil {
			log.Errorf("Error getting identity ID for %v: %v", id, err)
			continue
		}

		t.store.ProcessTagInfo([]*types.TagInfo{{
			Source:               "replay",
			Entity:               entityID,
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
func (t *Tagger) GetEntity(entityID string) (*types.Entity, error) {
	return t.store.GetEntity(entityID)
}
