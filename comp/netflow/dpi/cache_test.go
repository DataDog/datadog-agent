// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var exporter1 = []byte{10, 0, 0, 1}
var exporter2 = []byte{10, 0, 0, 2}

var browsingMetadata = Metadata{
	Category:            "browsing",
	SubCategory:         "other",
	ApplicationGroup:    "other",
	P2PTechnology:       "no",
	TunnelTechnology:    "no",
	EncryptedTechnology: "yes",
	TrafficClass:        "transactional-data",
	BusinessRelevance:   "default",
	ApplicationSet:      "general-browsing",
	ApplicationFamily:   "encrypted",
}

func httpRecord(id uint64) Record {
	return Record{
		Namespace:    "default",
		ExporterAddr: exporter1,
		Application:  Application{ID: id, Name: "HTTP", Description: "Hypertext Transfer Protocol"},
	}
}

func attributesRecord(id uint64) Record {
	return Record{
		Namespace:    "default",
		ExporterAddr: exporter1,
		Application:  Application{ID: id, Metadata: browsingMetadata},
	}
}

func TestApplicationCache_apply(t *testing.T) {
	tests := []struct {
		name    string
		records []Record
		check   func(t *testing.T, cache *ApplicationCache)
	}{
		{
			name:    "name and description are cached per exporter",
			records: []Record{httpRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				app, ok := cache.Lookup("default", exporter1, 100)
				assert.True(t, ok)
				assert.Equal(t, Application{ID: 100, Name: "HTTP", Description: "Hypertext Transfer Protocol"}, app)

				_, ok = cache.Lookup("default", exporter1, 101)
				assert.False(t, ok, "unrelated application ids must not resolve")

				_, ok = cache.Lookup("other-ns", exporter1, 100)
				assert.False(t, ok, "the same exporter in another namespace must not resolve")

				cache.MarkSeen("default", exporter2, 100)
				cache.apply([]Record{{Namespace: "default", ExporterAddr: exporter2, Application: Application{ID: 100, Name: "DNS"}}})

				app, ok = cache.Lookup("default", exporter2, 100)
				assert.True(t, ok, "same application id from a different exporter must resolve independently")
				assert.Equal(t, "DNS", app.Name)
				assert.Empty(t, app.Description)

				app, _ = cache.Lookup("default", exporter1, 100)
				assert.Equal(t, "HTTP", app.Name, "caching a second exporter must not disturb the first")
			},
		},
		{
			name:    "id 0 never resolves",
			records: []Record{httpRecord(0)},
			check: func(t *testing.T, cache *ApplicationCache) {
				_, ok := cache.Lookup("default", exporter1, 0)
				assert.False(t, ok)
			},
		},
		{
			name:    "attributes without a known name are not cached",
			records: []Record{attributesRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				_, ok := cache.Lookup("default", exporter1, 100)
				assert.False(t, ok)
			},
		},
		{
			name:    "attributes received before the name are dropped",
			records: []Record{attributesRecord(100), httpRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				app, _ := cache.Lookup("default", exporter1, 100)
				assert.Equal(t, "HTTP", app.Name)
				assert.Empty(t, app.Metadata)
			},
		},
		{
			name:    "attributes are kept when the same name is refreshed",
			records: []Record{httpRecord(100), attributesRecord(100), httpRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				app, _ := cache.Lookup("default", exporter1, 100)
				assert.Equal(t, browsingMetadata, app.Metadata)
			},
		},
		{
			name: "new name for a cached id evicts the previous application",
			records: []Record{
				httpRecord(100),
				attributesRecord(100),
				{
					Namespace:    "default",
					ExporterAddr: exporter1,
					Application:  Application{ID: 100, Name: "DNS", Metadata: Metadata{Category: "net-admin"}},
				},
			},
			check: func(t *testing.T, cache *ApplicationCache) {
				app, _ := cache.Lookup("default", exporter1, 100)
				assert.Equal(t, Application{ID: 100, Name: "DNS", Metadata: Metadata{Category: "net-admin"}}, app, "nothing from the previous application must carry over")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewApplicationCache()
			cache.MarkSeen("default", exporter1, 100)
			cache.apply(tt.records)
			tt.check(t, cache)
		})
	}
}

func TestApplicationCache_submit(t *testing.T) {
	cache := NewApplicationCache()
	cache.Start()
	defer cache.Stop()

	cache.MarkSeen("default", exporter1, 100)
	cache.Submit([]Record{httpRecord(100)})
	assert.Eventually(t, func() bool {
		_, ok := cache.Lookup("default", exporter1, 100)
		return ok
	}, time.Second, 10*time.Millisecond)
}

func TestApplicationCache_onlySeenApplications(t *testing.T) {
	cache := NewApplicationCache()
	cache.MarkSeen("default", exporter1, 100)
	cache.MarkSeen("default", exporter1, 0)
	cache.MarkSeen("other-ns", exporter1, 101)
	cache.MarkSeen("default", exporter2, 102)

	cache.apply([]Record{httpRecord(100), httpRecord(101), httpRecord(102), httpRecord(103)})

	_, ok := cache.Lookup("default", exporter1, 100)
	assert.True(t, ok)
	for _, id := range []uint64{101, 102, 103} {
		_, ok = cache.Lookup("default", exporter1, id)
		assert.False(t, ok, "id %d was never seen on a flow of this exporter in this namespace", id)
	}

	cache.MarkSeen("default", exporter1, 103)
	cache.apply([]Record{httpRecord(103)})
	_, ok = cache.Lookup("default", exporter1, 103)
	assert.True(t, ok, "an application is kept once its id has been seen")
}

func TestApplicationCache_submitDropsWhenFull(t *testing.T) {
	cache := NewApplicationCache() // not started, so nothing drains the queue
	for range recordsBufferSize {
		cache.Submit([]Record{httpRecord(100)})
	}
	assert.Zero(t, cache.DroppedRecords())

	cache.Submit([]Record{httpRecord(100), httpRecord(101)})
	assert.Equal(t, uint64(2), cache.DroppedRecords())
}

func TestApplicationCache_nil(t *testing.T) {
	var cache *ApplicationCache
	cache.Start()
	cache.MarkSeen("default", exporter1, 100)
	cache.Submit([]Record{httpRecord(100)})
	_, ok := cache.Lookup("default", exporter1, 100)
	assert.False(t, ok)
	assert.Zero(t, cache.DroppedRecords())
	cache.Stop()
}
