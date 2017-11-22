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
)

// Tagger is the entry class for entity tagging. It holds collectors, memory store
// and handles the query logic. One can use the package methods to use the default
// tagger instead of instanciating one.
type Tagger struct {
	sync.RWMutex
	tagStore    *tagStore
	pullers     map[string]collectors.Puller
	streamers   map[string]collectors.Streamer
	fetchers    map[string]collectors.Fetcher
	infoIn      chan []*collectors.TagInfo
	pullTicker  *time.Ticker
	pruneTicker *time.Ticker
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
		pullers:     make(map[string]collectors.Puller),
		streamers:   make(map[string]collectors.Streamer),
		fetchers:    make(map[string]collectors.Fetcher),
		infoIn:      make(chan []*collectors.TagInfo, 5),
		pullTicker:  time.NewTicker(5 * time.Second),
		pruneTicker: time.NewTicker(5 * time.Minute),
		stop:        make(chan bool),
	}

	return t, nil
}

// Init goes through a catalog and tries to detect which are relevant
// for this host. It then starts the collection logic and is ready for
// requests.
func (t *Tagger) Init(catalog collectors.Catalog) error {
	t.Lock()
	for name, factory := range catalog {
		log.Debugf("initialising tag collector %s", name)

		collector := factory()
		mode, err := collector.Detect(t.infoIn)
		if err != nil {
			log.Errorf("error initialising collector %s: %s", name, err)
			continue
		}
		switch mode {
		case collectors.PullCollection:
			pull, ok := collector.(collectors.Puller)
			if ok {
				t.pullers[name] = pull
				// TODO: schedule pulling
				t.fetchers[name] = pull
			} else {
				log.Errorf("error initialising collector %s: does not implement pull", name)
			}
		case collectors.StreamCollection:
			stream, ok := collector.(collectors.Streamer)
			if ok {
				t.streamers[name] = stream
				t.fetchers[name] = stream
				go stream.Stream()
			} else {
				log.Errorf("error initialising collector %s: does not implement stream", name)
			}
		case collectors.FetchOnlyCollection:
			fetch, ok := collector.(collectors.Fetcher)
			if ok {
				t.fetchers[name] = fetch
			} else {
				log.Errorf("error initialising collector %s: does not implement fetch", name)
			}
		}
	}
	t.Unlock()

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
			return nil
		case msg := <-t.infoIn:
			log.Debugf("listener message: %s", msg)
			for _, info := range msg {
				t.tagStore.processTagInfo(info)
			}
		case <-t.pullTicker.C:
			go t.pull()
		case <-t.pruneTicker.C:
			t.tagStore.prune()
		}
	}
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
