// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package tagger

import (
	"errors"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Tagger is the entry class for entity tagging. It holds collectors, memory store
// and handles the query logic. One can use the package methods to use the default
// tagger instead of instanciating one.
type Tagger struct {
	sync.RWMutex
	tagStore    *tagStore
	candidates  map[string]collectors.CollectorFactory
	pullers     map[string]collectors.Puller
	streamers   map[string]collectors.Streamer
	fetchers    map[string]collectors.Fetcher
	infoIn      chan []*collectors.TagInfo
	pullTicker  *time.Ticker
	pruneTicker *time.Ticker
	retryTicker *time.Ticker
	stop        chan bool
}

// newTagger returns an allocated tagger. You still have to run Init()
// once the config package is ready.
// You are probably looking for tagger.Tag() using the global instance
// instead of creating your own.
func newTagger() (*Tagger, error) {
	store, err := newTagStore()
	if err != nil {
		return nil, err
	}
	t := &Tagger{
		tagStore:    store,
		candidates:  make(map[string]collectors.CollectorFactory),
		pullers:     make(map[string]collectors.Puller),
		streamers:   make(map[string]collectors.Streamer),
		fetchers:    make(map[string]collectors.Fetcher),
		infoIn:      make(chan []*collectors.TagInfo, 5),
		pullTicker:  time.NewTicker(5 * time.Second),
		pruneTicker: time.NewTicker(5 * time.Minute),
		retryTicker: time.NewTicker(30 * time.Second),
		stop:        make(chan bool),
	}

	return t, nil
}

// Init goes through a catalog and tries to detect which are relevant
// for this host. It then starts the collection logic and is ready for
// requests.
func (t *Tagger) Init(catalog collectors.Catalog) error {
	t.Lock()
	// Populate collector candidate list from catalog
	// as we'll remove entries we need to copy the map
	for name, factory := range catalog {
		t.candidates[name] = factory
	}
	t.Unlock()

	log.Info("starting the tagging system")

	t.startCollectors()
	go t.run()
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
			return nil
		case msg := <-t.infoIn:
			log.Debugf("listener message: %s", msg)
			for _, info := range msg {
				t.tagStore.processTagInfo(info)
			}
		case <-t.retryTicker.C:
			go t.startCollectors()
		case <-t.pullTicker.C:
			go t.pull()
		case <-t.pruneTicker.C:
			t.tagStore.prune()
		}
	}
}

// startCollectors iterates over the listener candidates and tries initializing them.
// If the collector implements Retryer and return a FailWillRetry, we keep them in
// the map and will retry at the next tick.
func (t *Tagger) startCollectors() {
	t.RLock()
	if t.candidates == nil {
		log.Warnf("called with empty candidate map, skipping")
		t.RUnlock()
		return
	}
	replied := make(map[string]collectors.Collector)
	modes := make(map[string]collectors.CollectionMode)

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
		replied[name] = collector
		modes[name] = mode
	}
	t.RUnlock()

	t.Lock()
	for name, mode := range modes {
		// Whatever the outcome, don't try this collector again
		delete(t.candidates, name)

		collector, found := replied[name]
		if !found {
			log.Errorf("collector %s not found in replies, deleting it", name)
		}
		switch mode {
		case collectors.PullCollection:
			pull, ok := collector.(collectors.Puller)
			if ok {
				t.pullers[name] = pull
				t.fetchers[name] = pull
			} else {
				log.Errorf("error initializing collector %s: does not implement pull", name)
			}
		case collectors.StreamCollection:
			stream, ok := collector.(collectors.Streamer)
			if ok {
				t.streamers[name] = stream
				t.fetchers[name] = stream
				go stream.Stream()
			} else {
				log.Errorf("error initializing collector %s: does not implement stream", name)
			}
		case collectors.FetchOnlyCollection:
			fetch, ok := collector.(collectors.Fetcher)
			if ok {
				t.fetchers[name] = fetch
			} else {
				log.Errorf("error initializing collector %s: does not implement fetch", name)
			}
		}
	}

	if len(t.candidates) == 0 {
		log.Debugf("candidate list empty, stopping detection")
		t.retryTicker.Stop()
		t.candidates = nil
	}
	t.Unlock()
}

func (t *Tagger) pull() {
	t.RLock()
	for _, puller := range t.pullers {
		err := puller.Pull()
		if err != nil {
			log.Warnf("%s", err.Error())
		}
	}
	t.RUnlock()
}

// Stop queues a shutdown of Tagger
func (t *Tagger) Stop() error {
	t.stop <- true
	return nil
}

// Tag returns tags for a given entity. If highCard is false, high
// cardinality tags are left out.
func (t *Tagger) Tag(entity string, highCard bool) ([]string, error) {
	if entity == "" {
		return nil, errors.New("empty entity ID")
	}
	cachedTags, sources, err := t.tagStore.lookup(entity, highCard)
	if err != nil {
		return nil, err
	}
	if len(sources) == len(t.fetchers) {
		// All sources sent data to cache
		return cachedTags, nil
	}
	// Else, partial cache miss, query missing data
	// TODO: get logging on that to make sure we should optimize
	tagArrays := [][]string{cachedTags}

	t.RLock()
ITER_COLLECTORS:
	for name, collector := range t.fetchers {
		for _, s := range sources {
			if s == name {
				continue ITER_COLLECTORS // source was in cache, don't lookup again
			}
		}
		log.Debugf("cache miss for %s, collecting", name)
		low, high, err := collector.Fetch(entity)
		if err != nil {
			log.Warnf("error collecting from %s: %s", name, err)
			// FIXME: introduce a custom error type
			if !strings.Contains(err.Error(), "not found in") {
				// We want to store the empty tag response if the error
				// comes from successfully parsing the source but not
				// finding the entity (can happen in k8s/ECS in pure
				// docker containers).
				// On other cases, don't store that to retry on next query.
				continue
			}
		}
		tagArrays = append(tagArrays, low)
		if highCard {
			tagArrays = append(tagArrays, high)
		}
		// Submit to cache for next lookup
		t.tagStore.processTagInfo(&collectors.TagInfo{
			Entity:       entity,
			Source:       name,
			LowCardTags:  low,
			HighCardTags: high,
		})
	}
	t.RUnlock()

	return utils.ConcatenateTags(tagArrays), nil
}
