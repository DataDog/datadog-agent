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

	"go.uber.org/atomic"
)

// ApplicationIDField is the additional field goflowlib maps the flow's applicationId (IANA 95) to
const ApplicationIDField = "datadog.application_id"

// recordsBufferSize is the number of options packets that can be queued before dropping them
const recordsBufferSize = 100

// Application is the application an exporter announced for an application id
type Application struct {
	ID          uint64
	Name        string
	Description string
	Metadata
}

// Metadata holds optional application attributes, e.g. from NBAR's `option application-attributes`
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

// Record is an application decoded from an options record, along with the exporter that sent it
type Record struct {
	Namespace    string
	ExporterAddr []byte
	Application
}

type exporterKey struct {
	namespace    string
	exporterAddr string
}

// ApplicationCache holds the applications announced by each exporter.
// Records are submitted by listeners and applied asynchronously, so that flow decoding never waits on the cache.
// Only applications whose id was seen on a flow are kept, instead of the exporter's full table.
// A nil *ApplicationCache is valid and ignores records and lookups (DPI disabled).
type ApplicationCache struct {
	mu   sync.RWMutex
	apps map[exporterKey]map[uint64]Application

	seenMu sync.RWMutex
	seen   map[exporterKey]map[uint64]struct{}

	records        chan []Record
	stop           chan struct{}
	done           chan struct{}
	droppedRecords *atomic.Uint64
}

// NewApplicationCache returns a new ApplicationCache, which must be started to apply records
func NewApplicationCache() *ApplicationCache {
	return &ApplicationCache{
		apps:           make(map[exporterKey]map[uint64]Application),
		seen:           make(map[exporterKey]map[uint64]struct{}),
		records:        make(chan []Record, recordsBufferSize),
		stop:           make(chan struct{}),
		done:           make(chan struct{}),
		droppedRecords: atomic.NewUint64(0),
	}
}

// Start applies submitted records until Stop is called
func (c *ApplicationCache) Start() {
	if c == nil {
		return
	}
	go func() {
		defer close(c.done)
		for {
			select {
			case <-c.stop:
				return
			case records := <-c.records:
				c.apply(records)
			}
		}
	}()
}

// Stop stops applying records
func (c *ApplicationCache) Stop() {
	if c == nil {
		return
	}
	close(c.stop)
	<-c.done
}

// Submit queues records to be applied, dropping them if the queue is full
func (c *ApplicationCache) Submit(records []Record) {
	if c == nil || len(records) == 0 {
		return
	}
	select {
	case c.records <- records:
	default:
		c.droppedRecords.Add(uint64(len(records)))
	}
}

// DroppedRecords returns the number of records dropped because the queue was full
func (c *ApplicationCache) DroppedRecords() uint64 {
	if c == nil {
		return 0
	}
	return c.droppedRecords.Load()
}

// MarkSeen records that the exporter reported the application id on a flow
func (c *ApplicationCache) MarkSeen(namespace string, exporterAddr []byte, id uint64) {
	if c == nil || id == 0 {
		return
	}
	key := exporterKey{namespace: namespace, exporterAddr: string(exporterAddr)}
	if c.isSeen(key, id) {
		return
	}

	c.seenMu.Lock()
	defer c.seenMu.Unlock()
	ids := c.seen[key]
	if ids == nil {
		ids = make(map[uint64]struct{})
		c.seen[key] = ids
	}
	ids[id] = struct{}{}
}

func (c *ApplicationCache) isSeen(key exporterKey, id uint64) bool {
	c.seenMu.RLock()
	defer c.seenMu.RUnlock()
	_, ok := c.seen[key][id]
	return ok
}

// Lookup returns the application an exporter announced for an application id
func (c *ApplicationCache) Lookup(namespace string, exporterAddr []byte, id uint64) (Application, bool) {
	if c == nil || id == 0 {
		return Application{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	app, ok := c.apps[exporterKey{namespace: namespace, exporterAddr: string(exporterAddr)}][id]
	return app, ok
}

func (c *ApplicationCache) apply(records []Record) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, record := range records {
		c.applyRecord(record)
	}
}

func (c *ApplicationCache) applyRecord(record Record) {
	key := exporterKey{namespace: record.Namespace, exporterAddr: string(record.ExporterAddr)}
	if !c.isSeen(key, record.ID) {
		return
	}
	apps := c.apps[key]
	app, known := apps[record.ID]
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

	if apps == nil {
		apps = make(map[uint64]Application)
		c.apps[key] = apps
	}
	apps[record.ID] = app
}
