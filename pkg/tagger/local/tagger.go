// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/tagstore"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Tagger is the entry class for entity tagging. It holds collectors, memory store
// and handles the query logic. One can use the package methods to use the default
// Tagger instead of instantiating one.
type Tagger struct {
	sync.RWMutex
	candidates map[string]collectors.CollectorFactory
	pullers    map[string]collectors.Puller
	streamers  map[string]collectors.Streamer

	store       *tagstore.TagStore
	retryTicker *time.Ticker

	ctx    context.Context
	cancel context.CancelFunc
}

type collectorReply struct {
	name     string
	mode     collectors.CollectionMode
	instance collectors.Collector
}

// NewTagger returns an allocated tagger. You still have to run Init()
// once the config package is ready.
// You are probably looking for tagger.Tag() using the global instance
// instead of creating your own.
func NewTagger(catalog collectors.Catalog) *Tagger {
	ctx, cancel := context.WithCancel(context.TODO())
	t := &Tagger{
		candidates: make(map[string]collectors.CollectorFactory),
		pullers:    make(map[string]collectors.Puller),
		streamers:  make(map[string]collectors.Streamer),

		store: tagstore.NewTagStore(),

		ctx:    ctx,
		cancel: cancel,
	}

	// Populate collector candidate list from catalog
	// as we'll remove entries we need to copy the map
	for name, factory := range catalog {
		t.candidates[name] = factory
	}

	return t
}

// Init goes through a catalog and tries to detect which are relevant
// for this host. It then starts the collection logic and is ready for
// requests.
func (t *Tagger) Init() error {
	t.retryTicker = time.NewTicker(30 * time.Second)

	t.startCollectors(t.ctx)

	go t.runPuller(t.ctx)
	go t.store.Run(t.ctx)

	go t.run()

	return nil
}

func (t *Tagger) run() {
	for {
		select {
		case <-t.retryTicker.C:
			t.startCollectors(t.ctx)

		case <-t.ctx.Done():
			// NOTE: in the future, we could pass a context to
			// streamers so they can stop themselves.
			t.RLock()
			for name, collector := range t.streamers {
				err := collector.Stop()
				if err != nil {
					log.Warnf("error stopping %s: %s", name, err)
				}
			}
			t.RUnlock()

			t.retryTicker.Stop()

			return
		}
	}
}

func (t *Tagger) runPuller(ctx context.Context) {
	pullTicker := time.NewTicker(5 * time.Second)
	health := health.RegisterLiveness("tagger-pull")

	for {
		select {
		case <-pullTicker.C:
			t.pull(ctx)

		case <-health.C:

		case <-ctx.Done():
			pullTicker.Stop()

			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}

			return
		}
	}
}

// startCollectors iterates over the listener candidates and tries initializing them.
// If the collector implements Retryer and return a FailWillRetry, we keep them in
// the map and will retry at the next tick.
func (t *Tagger) startCollectors(ctx context.Context) {
	replies := t.tryCollectors(ctx)
	if len(replies) > 0 {
		t.registerCollectors(replies)
	}
	if len(t.candidates) == 0 {
		log.Debugf("candidate list empty, stopping detection")
		t.retryTicker.Stop()
	}
}

func (t *Tagger) tryCollectors(ctx context.Context) []collectorReply {
	t.Lock()
	defer t.Unlock()

	if t.candidates == nil {
		log.Warnf("called with empty candidate map, skipping")
		return nil
	}
	var replies []collectorReply

	for name, factory := range t.candidates {
		collector := factory()
		mode, err := collector.Detect(ctx, t.store.InfoIn)
		if mode == collectors.NoCollection && err == nil {
			log.Infof("collector %s skipped as feature not activated", name)
			delete(t.candidates, name)
			continue
		}
		if retry.IsErrWillRetry(err) {
			log.Debugf("will retry %s later: %s", name, err)
			continue // don't add it to the modes map as we want to retry later
		}
		if err != nil {
			log.Debugf("%s tag collector cannot start: %s", name, err)
		} else {
			log.Infof("%s tag collector successfully started", name)
		}
		replies = append(replies, collectorReply{
			name:     name,
			mode:     mode,
			instance: collector,
		})
	}

	return replies
}

func (t *Tagger) registerCollectors(replies []collectorReply) {
	t.Lock()
	for _, c := range replies {
		// Whatever the outcome, don't try this collector again
		delete(t.candidates, c.name)

		switch c.mode {
		case collectors.PullCollection:
			pull, ok := c.instance.(collectors.Puller)
			if ok {
				t.pullers[c.name] = pull
			} else {
				log.Errorf("error initializing collector %s: does not implement pull", c.name)
			}
		case collectors.StreamCollection:
			stream, ok := c.instance.(collectors.Streamer)
			if ok {
				t.streamers[c.name] = stream
				go stream.Stream() //nolint:errcheck
			} else {
				log.Errorf("error initializing collector %s: does not implement stream", c.name)
			}
		}
	}
	t.Unlock()
}

func (t *Tagger) pull(ctx context.Context) {
	t.RLock()
	for name, puller := range t.pullers {
		err := puller.Pull(ctx)
		if err != nil {
			log.Warnf("Error pulling from %s: %s", name, err.Error())
		}
	}
	t.RUnlock()
}

// Stop queues a shutdown of Tagger
func (t *Tagger) Stop() error {
	t.cancel()
	return nil
}

// getTags returns a read only list of tags for a given entity.
func (t *Tagger) getTags(entity string, cardinality collectors.TagCardinality) (tagset.HashedTags, error) {
	if entity == "" {
		telemetry.QueriesByCardinality(cardinality).EmptyEntityID.Inc()
		return tagset.HashedTags{}, fmt.Errorf("empty entity ID")
	}

	cachedTags := t.store.LookupHashed(entity, cardinality)

	telemetry.QueriesByCardinality(cardinality).Success.Inc()
	return cachedTags, nil
}

// AccumulateTagsFor appends tags for a given entity from the tagger to the TagAccumulator
func (t *Tagger) AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagAccumulator) error {
	tags, err := t.getTags(entity, cardinality)
	tb.AppendHashed(tags)
	return err
}

// Tag returns a copy of the tags for a given entity
func (t *Tagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	tags, err := t.getTags(entity, cardinality)
	if err != nil {
		return nil, err
	}
	return tags.Copy(), nil
}

// Standard returns standard tags for a given entity
// It triggers a tagger fetch if the no tags are found
func (t *Tagger) Standard(entity string) ([]string, error) {
	if entity == "" {
		return nil, fmt.Errorf("empty entity ID")
	}

	tags, err := t.store.LookupStandard(entity)
	if err == tagstore.ErrNotFound {
		// entity not found yet in the tagger
		// trigger tagger fetch operations
		log.Debugf("Entity '%s' not found in tagger cache, will try to fetch it", entity)
		_, _ = t.Tag(entity, collectors.LowCardinality)

		return t.store.LookupStandard(entity)
	}

	if err != nil {
		return nil, fmt.Errorf("Entity %q not found: %w", entity, err)
	}

	return tags, nil
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *Tagger) GetEntity(entityID string) (*types.Entity, error) {
	return t.store.GetEntity(entityID)
}

// List the content of the tagger
func (t *Tagger) List(cardinality collectors.TagCardinality) response.TaggerListResponse {
	return t.store.List()
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (t *Tagger) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	return t.store.Subscribe(cardinality)
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (t *Tagger) Unsubscribe(ch chan []types.EntityEvent) {
	t.store.Unsubscribe(ch)
}
