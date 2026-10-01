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

func httpRecord(id uint64) ApplicationRecord {
	return ApplicationRecord{
		Namespace:    "default",
		ExporterAddr: exporter1,
		Application:  Application{ID: id, Name: "HTTP", Description: "Hypertext Transfer Protocol"},
	}
}

func attributesRecord(id uint64) ApplicationRecord {
	return ApplicationRecord{
		Namespace:    "default",
		ExporterAddr: exporter1,
		Application:  Application{ID: id, Metadata: browsingMetadata},
	}
}

func TestApplicationCache_apply(t *testing.T) {
	tests := []struct {
		name    string
		records []ApplicationRecord
		check   func(t *testing.T, cache *ApplicationCache)
	}{
		{
			name:    "name and description are cached per exporter",
			records: []ApplicationRecord{httpRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				app, ok := cache.Lookup("default", exporter1, 100)
				assert.True(t, ok)
				assert.Equal(t, Application{ID: 100, Name: "HTTP", Description: "Hypertext Transfer Protocol"}, app)

				_, ok = cache.Lookup("default", exporter1, 101)
				assert.False(t, ok, "unrelated application ids must not resolve")

				_, ok = cache.Lookup("other-ns", exporter1, 100)
				assert.False(t, ok, "the same exporter in another namespace must not resolve")

				cache.Submit([]ApplicationRecord{{Namespace: "default", ExporterAddr: exporter2, Application: Application{ID: 100, Name: "DNS"}}})

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
			records: []ApplicationRecord{httpRecord(0)},
			check: func(t *testing.T, cache *ApplicationCache) {
				_, ok := cache.Lookup("default", exporter1, 0)
				assert.False(t, ok)
			},
		},
		{
			name:    "attributes without a known name are not cached",
			records: []ApplicationRecord{attributesRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				_, ok := cache.Lookup("default", exporter1, 100)
				assert.False(t, ok)
			},
		},
		{
			name:    "attributes received before the name are dropped",
			records: []ApplicationRecord{attributesRecord(100), httpRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				app, _ := cache.Lookup("default", exporter1, 100)
				assert.Equal(t, "HTTP", app.Name)
				assert.Empty(t, app.Metadata)
			},
		},
		{
			name:    "attributes are kept when the same name is refreshed",
			records: []ApplicationRecord{httpRecord(100), attributesRecord(100), httpRecord(100)},
			check: func(t *testing.T, cache *ApplicationCache) {
				app, _ := cache.Lookup("default", exporter1, 100)
				assert.Equal(t, browsingMetadata, app.Metadata)
			},
		},
		{
			name: "new name for a cached id evicts the previous application",
			records: []ApplicationRecord{
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
			cache.Submit(tt.records)
			tt.check(t, cache)
		})
	}
}

func TestApplicationCache_ignoresZeroID(t *testing.T) {
	cache := NewApplicationCache()
	cache.Submit([]ApplicationRecord{httpRecord(0)})
	_, ok := cache.apps.Peek(applicationKey{namespace: "default", exporterAddr: string(exporter1), id: 0})
	assert.False(t, ok)
}

func TestApplicationCache_evictsLeastRecentlyUsed(t *testing.T) {
	cache := newApplicationCache(2, time.Hour)
	cache.Submit([]ApplicationRecord{httpRecord(100), httpRecord(101)})
	cache.Lookup("default", exporter1, 100)
	cache.Submit([]ApplicationRecord{httpRecord(102)})

	_, ok := cache.Lookup("default", exporter1, 101)
	assert.False(t, ok, "the least recently used application is evicted")
	_, ok = cache.Lookup("default", exporter1, 100)
	assert.True(t, ok)
	_, ok = cache.Lookup("default", exporter1, 102)
	assert.True(t, ok)
}

func TestApplicationCache_expiresUnannouncedApplications(t *testing.T) {
	cache := newApplicationCache(10, 50*time.Millisecond)
	cache.Submit([]ApplicationRecord{httpRecord(100), httpRecord(101)})

	assert.Eventually(t, func() bool {
		cache.Submit([]ApplicationRecord{httpRecord(100)}) // the exporter keeps announcing 100
		_, ok := cache.Lookup("default", exporter1, 101)
		return !ok
	}, time.Second, 10*time.Millisecond, "an application the exporter stops announcing expires")
	_, ok := cache.Lookup("default", exporter1, 100)
	assert.True(t, ok, "re-announcing an application refreshes its TTL")
}
