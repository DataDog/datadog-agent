// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Tagger is the entry class for entity tagging. It holds collectors, memory store
// and handles the query logic. One can use the package methods to use the default
// Tagger instead of instantiating one.
type Tagger struct {
	sync.RWMutex
	store           *tagStore
	candidates      map[string]collectors.CollectorFactory
	pullers         map[string]collectors.Puller
	streamers       map[string]collectors.Streamer
	fetchers        map[string]collectors.Fetcher
	infoIn          chan []*collectors.TagInfo
	pullTicker      *time.Ticker
	pruneTicker     *time.Ticker
	retryTicker     *time.Ticker
	telemetryTicker *time.Ticker
	stop            chan bool
	health          *health.Handle
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
	t := &Tagger{
		store:           newTagStore(),
		candidates:      make(map[string]collectors.CollectorFactory),
		pullers:         make(map[string]collectors.Puller),
		streamers:       make(map[string]collectors.Streamer),
		fetchers:        make(map[string]collectors.Fetcher),
		infoIn:          make(chan []*collectors.TagInfo, 5),
		pullTicker:      time.NewTicker(5 * time.Second),
		pruneTicker:     time.NewTicker(5 * time.Minute),
		retryTicker:     time.NewTicker(30 * time.Second),
		telemetryTicker: time.NewTicker(1 * time.Minute),
		stop:            make(chan bool),
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
	// Only register the health check when the tagger is started
	t.health = health.RegisterLiveness("tagger")

	t.startCollectors()
	go t.run() //nolint:errcheck
	go t.pull()

	return nil
}

func (t *Tagger) run() error {
	for {
		select {
		case <-t.stop:
			t.RLock()
			for name, collector := range t.streamers {
				err := collector.Stop()
				if err != nil {
					log.Warnf("error stopping %s: %s", name, err)
				}
			}
			t.RUnlock()
			t.pullTicker.Stop()
			t.pruneTicker.Stop()
			t.retryTicker.Stop()
			t.telemetryTicker.Stop()
			t.health.Deregister() //nolint:errcheck
			return nil
		case <-t.health.C:
		case msg := <-t.infoIn:
			t.store.processTagInfo(msg)
		case <-t.retryTicker.C:
			go t.startCollectors()
		case <-t.pullTicker.C:
			go t.pull()
		case <-t.pruneTicker.C:
			t.store.prune()
		case <-t.telemetryTicker.C:
			t.store.collectTelemetry()
		}
	}
}

// startCollectors iterates over the listener candidates and tries initializing them.
// If the collector implements Retryer and return a FailWillRetry, we keep them in
// the map and will retry at the next tick.
func (t *Tagger) startCollectors() {
	replies := t.tryCollectors()
	if len(replies) > 0 {
		t.registerCollectors(replies)
	}
	if len(t.candidates) == 0 {
		log.Debugf("candidate list empty, stopping detection")
		t.retryTicker.Stop()
	}
}

func (t *Tagger) tryCollectors() []collectorReply {
	t.RLock()
	if t.candidates == nil {
		log.Warnf("called with empty candidate map, skipping")
		t.RUnlock()
		return nil
	}
	var replies []collectorReply

	for name, factory := range t.candidates {
		collector := factory()
		mode, err := collector.Detect(t.infoIn)
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
	t.RUnlock()
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
				t.fetchers[c.name] = pull
			} else {
				log.Errorf("error initializing collector %s: does not implement pull", c.name)
			}
		case collectors.StreamCollection:
			stream, ok := c.instance.(collectors.Streamer)
			if ok {
				t.streamers[c.name] = stream
				t.fetchers[c.name] = stream
				go stream.Stream() //nolint:errcheck
			} else {
				log.Errorf("error initializing collector %s: does not implement stream", c.name)
			}
		case collectors.FetchOnlyCollection:
			fetch, ok := c.instance.(collectors.Fetcher)
			if ok {
				t.fetchers[c.name] = fetch
			} else {
				log.Errorf("error initializing collector %s: does not implement fetch", c.name)
			}
		}
	}
	t.Unlock()
}

func (t *Tagger) pull() {
	t.RLock()
	for name, puller := range t.pullers {
		err := puller.Pull()
		if err != nil {
			log.Warnf("Error pulling from %s: %s", name, err.Error())
		}
	}
	t.RUnlock()
}

// Stop queues a shutdown of Tagger
func (t *Tagger) Stop() error {
	t.stop <- true
	return nil
}

// getTags returns a read only list of tags for a given entity.
func (t *Tagger) getTags(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	telemetry.Queries.Inc(collectors.TagCardinalityToString(cardinality))

	if entity == "" {
		return nil, fmt.Errorf("empty entity ID")
	}
	cachedTags, sources := t.store.lookup(entity, cardinality)

	if len(sources) == len(t.fetchers) {
		// All sources sent data to cache
		return cachedTags, nil
	}
	// Else, partial cache miss, query missing data
	// TODO: get logging on that to make sure we should optimize
	tagArrays := [][]string{cachedTags}

	t.RLock()
IterCollectors:
	for name, collector := range t.fetchers {
		for _, s := range sources {
			if s == name {
				continue IterCollectors // source was in cache, don't lookup again
			}
		}

		log.Debugf("cache miss for %s, collecting tags for %s", name, entity)

		cacheMiss := false
		skipCache := false
		low, orch, high, err := collector.Fetch(entity)
		switch {
		case errors.IsNotFound(err):
			log.Debugf("entity %s not found in %s, skipping: %v", entity, name, err)
			cacheMiss = true
		case errors.IsPartial(err):
			skipCache = true
		case err != nil:
			log.Warnf("error collecting from %s: %s", name, err)
			continue // don't store empty tags, retry next time
		}

		tagArrays = append(tagArrays, low)
		if cardinality == collectors.OrchestratorCardinality {
			tagArrays = append(tagArrays, orch)
		} else if cardinality == collectors.HighCardinality {
			tagArrays = append(tagArrays, orch)
			tagArrays = append(tagArrays, high)
		}

		// Submit to cache for next lookup
		t.store.processTagInfo([]*collectors.TagInfo{
			{
				Entity:               entity,
				Source:               name,
				LowCardTags:          low,
				OrchestratorCardTags: orch,
				HighCardTags:         high,
				CacheMiss:            cacheMiss,
				SkipCache:            skipCache,
			},
		})
	}
	t.RUnlock()

	return utils.ConcatenateTags(tagArrays), nil
}

// TagBuilder appends tags for a given entity from the tagger to the TagsBuilder
func (t *Tagger) TagBuilder(entity string, cardinality collectors.TagCardinality, tb *util.TagsBuilder) error {
	tags, err := t.getTags(entity, cardinality)
	if err == nil {
		tb.Append(tags...)
	}
	return err
}

// Tag returns a copy of the tags for a given entity
func (t *Tagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	tags, err := t.getTags(entity, cardinality)
	if err != nil {
		return nil, err
	}

	return copyArray(tags), nil
}

// Standard returns standard tags for a given entity
// It triggers a tagger fetch if the no tags are found
func (t *Tagger) Standard(entity string) ([]string, error) {
	if entity == "" {
		return nil, fmt.Errorf("empty entity ID")
	}

	tags, err := t.store.lookupStandard(entity)
	if err == errNotFound {
		// entity not found yet in the tagger
		// trigger tagger fetch operations
		log.Debugf("Entity '%s' not found in tagger cache, will try to fetch it", entity)
		_, _ = t.Tag(entity, collectors.LowCardinality)

		return t.store.lookupStandard(entity)
	}

	if err != nil {
		return nil, fmt.Errorf("Entity %q not found: %w", entity, err)
	}

	return tags, nil
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *Tagger) GetEntity(entityID string) (*types.Entity, error) {

	tags, err := t.store.getEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	entity := tags.toEntity()
	return &entity, nil
}

// List the content of the tagger
func (t *Tagger) List(cardinality collectors.TagCardinality) response.TaggerListResponse {
	r := response.TaggerListResponse{
		Entities: make(map[string]response.TaggerListEntity),
	}

	t.store.RLock()
	defer t.store.RUnlock()

	for entityID, et := range t.store.store {
		entity := response.TaggerListEntity{
			Tags: make(map[string][]string),
		}

		for source, sourceTags := range et.sourceTags {
			tags := append([]string(nil), sourceTags.lowCardTags...)
			tags = append(tags, sourceTags.orchestratorCardTags...)
			tags = append(tags, sourceTags.highCardTags...)
			entity.Tags[source] = tags
		}

		r.Entities[entityID] = entity
	}

	return r
}

// Subscribe returns a list of existing entities in the store, alongside a
// channel that receives events whenever an entity is added, modified or
// deleted.
func (t *Tagger) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	return t.store.subscribe(cardinality)
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (t *Tagger) Unsubscribe(ch chan []types.EntityEvent) {
	t.store.unsubscribe(ch)
}

// copyArray makes sure the tagger does not return internal slices
// that could be modified by others, by explicitly copying the slice
// contents to a new slice. As strings are references, the size of
// the new array is small enough.
func copyArray(source []string) []string {
	copied := make([]string, len(source))
	copy(copied, source)
	return copied
}
