// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package dpi resolves the application ids reported by exporters on flows into
// the applications they announce through options records.
// Decoding options records is left to goflowlib, this package only handles typed values.
package dpi

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

const ApplicationIDField = "datadog.application_id"

type Application struct {
	ID          uint64
	Name        string
	Description string
	Metadata
}

type Metadata struct {
	Category            string
	SubCategory         string
	ApplicationGroup    string
	P2PTechnology       string
	TunnelTechnology    string
	EncryptedTechnology string
	TrafficClass        string
	BusinessRelevance   string
	ApplicationSet      string
	ApplicationFamily   string
}

type ApplicationRecord struct {
	Namespace    string
	ExporterAddr []byte
	Application
}

const applicationTTL = time.Hour
const maxApplications = 100_000

type applicationKey struct {
	namespace    string
	exporterAddr string
	id           uint64
}

// Cache resolves the application ids reported on flows into the applications announced by their exporter
type Cache interface {
	// updates the cache with the applications decoded from options records
	Submit(records []ApplicationRecord)
	// returns the application an exporter announced for an application id
	Lookup(namespace string, exporterAddr []byte, id uint64) (Application, bool)
}

type ApplicationCache struct {
	mu   sync.Mutex
	apps *expirable.LRU[applicationKey, Application]
}

func NewApplicationCache() *ApplicationCache {
	return newApplicationCache(maxApplications, applicationTTL)
}

func newApplicationCache(size int, ttl time.Duration) *ApplicationCache {
	return &ApplicationCache{
		apps: expirable.NewLRU[applicationKey, Application](size, nil, ttl),
	}
}

func (c *ApplicationCache) Submit(records []ApplicationRecord) {
	if len(records) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, record := range records {
		c.addToCache(record)
	}
}

func (c *ApplicationCache) Lookup(namespace string, exporterAddr []byte, id uint64) (Application, bool) {
	if id == 0 {
		return Application{}, false
	}
	return c.apps.Get(applicationKey{namespace: namespace, exporterAddr: string(exporterAddr), id: id})
}

func (c *ApplicationCache) addToCache(record ApplicationRecord) {
	if record.ID == 0 {
		return
	}
	key := applicationKey{namespace: record.Namespace, exporterAddr: string(record.ExporterAddr), id: record.ID}
	app, known := c.apps.Peek(key)
	if record.Name != "" {
		if record.Name != app.Name {
			// the id now maps to a different application, so drop the old name, description and metadata
			app = Application{ID: record.ID, Name: record.Name}
		}
		app.Description = record.Description
		known = true
	}
	if !known {
		// never cache metadata for an id without a name
		return
	}
	// only replace metadata when the record has some, so name-only records keep it
	if record.Metadata != (Metadata{}) {
		app.Metadata = record.Metadata
	}
	// adding refreshes the application's TTL
	c.apps.Add(key, app)
}
