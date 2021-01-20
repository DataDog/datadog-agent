// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package local

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

const (
	batchSize = 100
	nMaxTags  = 5
	nSources  = 10
	nEntities = 1000
)

var (
	sources []string
	ids     []string
)

func init() {
	sources = make([]string, 0, nSources)
	for i := 0; i < nSources; i++ {
		sources = append(sources, fmt.Sprintf("source_%d", i))
	}

	ids = make([]string, 0, nEntities)
	for i := 0; i < nEntities; i++ {
		ids = append(ids, strconv.FormatInt(rand.Int63(), 16))
	}
}

func BenchmarkTagStoreThroughput(b *testing.B) {
	store := newTagStore()

	doneCh := make(chan struct{})
	pruneTicker := time.NewTicker(time.Second)

	go func() {
		select {
		case <-pruneTicker.C:
			store.prune()
		case <-doneCh:
			return
		}
	}()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			for i := 0; i < 100; i++ {
				processRandomTagInfoBatch(store)
			}
			wg.Done()
		}()

		go func() {
			for i := 0; i < 1000; i++ {
				id := ids[rand.Intn(nEntities)]
				store.lookup(id, collectors.HighCardinality)
			}
			wg.Done()
		}()

		wg.Wait()
	}

	close(doneCh)
}

// BenchmarkTagStore_processTagInfo benchmarks how fast the tagStore can ingest
// changes to entities. It does not do so concurrently, as even though the
// store is thread-safe, processTagInfo is always used synchronously by the
// tagger at the moment.
func BenchmarkTagStore_processTagInfo(b *testing.B) {
	store := newTagStore()

	for i := 0; i < b.N; i++ {
		processRandomTagInfoBatch(store)
	}
}

func generateRandomTagInfo() *collectors.TagInfo {
	id := ids[rand.Intn(nEntities)]
	source := sources[rand.Intn(nSources)]
	return &collectors.TagInfo{
		Entity:               id,
		Source:               source,
		LowCardTags:          generateRandomTags(),
		OrchestratorCardTags: generateRandomTags(),
		HighCardTags:         generateRandomTags(),
		StandardTags:         generateRandomTags(),
	}
}

func generateRandomTags() []string {
	nTags := rand.Intn(nMaxTags)
	tags := make([]string, 0, nTags)
	for i := 0; i < nTags; i++ {
		tags = append(tags, strconv.FormatInt(rand.Int63(), 16))
	}

	return tags
}

func processRandomTagInfoBatch(store *tagStore) {
	tagInfos := make([]*collectors.TagInfo, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		tagInfos = append(tagInfos, generateRandomTagInfo())
	}

	store.processTagInfo(tagInfos)
}
