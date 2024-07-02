// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pathteststore handle pathtest storage
package pathteststore

import (
	"sync"
	time "time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
)

var timeNow = time.Now

// PathtestContext contains Pathtest information and additional flush related data
type PathtestContext struct {
	Pathtest *common.Pathtest

	nextRun           time.Time
	runUntil          time.Time
	lastFlushTime     time.Time
	lastFlushInterval time.Duration
}

// LastFlushInterval returns last flush interval
func (p *PathtestContext) LastFlushInterval() time.Duration {
	return p.lastFlushInterval
}

// SetLastFlushInterval sets last flush interval
func (p *PathtestContext) SetLastFlushInterval(lastFlushInterval time.Duration) {
	p.lastFlushInterval = lastFlushInterval
}

// Store is used to accumulate aggregated contexts
type Store struct {
	logger log.Component

	contexts map[uint64]*PathtestContext

	// mutex is needed to protect `contexts` since `Store.add()` and  `pathtestStore.flush()`
	// are called by different routines.
	contextsMutex sync.Mutex

	// interval defines how frequently pathtests should run
	interval time.Duration

	// ttl is the duration a Pathtest should run from discovery.
	// If a Pathtest is added again before the TTL expires, the TTL is reset to this duration.
	ttl time.Duration
}

func newPathtestContext(pt *common.Pathtest, runUntilDuration time.Duration) *PathtestContext {
	now := timeNow()
	return &PathtestContext{
		Pathtest: pt,
		nextRun:  now,
		runUntil: now.Add(runUntilDuration),
	}
}

// NewPathtestStore creates a new Store
func NewPathtestStore(pathtestTTL time.Duration, pathtestInterval time.Duration, logger log.Component) *Store {
	return &Store{
		contexts: make(map[uint64]*PathtestContext),
		ttl:      pathtestTTL,
		interval: pathtestInterval,
		logger:   logger,
	}
}

// Flush will flush specific Pathtest context (distinct hash) if nextRun is reached
// once a Pathtest context is flushed nextRun will be updated to the next flush time
//
// ttl:
// ttl defines the duration we should keep a specific PathtestContext in `Store.contexts`
// after `lastSuccessfulFlush`. // Flow context in `Store.contexts` map will be deleted if `ttl`
// is reached to avoid keeping Pathtest context that are not seen anymore.
// We need to keep PathtestContext (contains `nextRun` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flushInterval`).
// Example, after a flush, PathtestContext will have a new nextRun, that will be the next flush time for new contexts being added.
func (f *Store) Flush() []*PathtestContext {
	f.contextsMutex.Lock()
	defer f.contextsMutex.Unlock()

	f.logger.Tracef("f.contexts: %+v", f.contexts)

	var pathtestsToFlush []*PathtestContext
	for key, ptConfigCtx := range f.contexts {
		now := timeNow()

		if ptConfigCtx.runUntil.Before(now) {
			f.logger.Tracef("Delete Pathtest context (key=%d, runUntil=%s, nextRun=%s)", key, ptConfigCtx.runUntil, ptConfigCtx.nextRun)
			// delete ptConfigCtx wrapper if it reaches runUntil
			delete(f.contexts, key)
			continue
		}
		if ptConfigCtx.nextRun.After(now) {
			continue
		}
		if !ptConfigCtx.lastFlushTime.IsZero() {
			ptConfigCtx.lastFlushInterval = now.Sub(ptConfigCtx.lastFlushTime)
		}
		ptConfigCtx.lastFlushTime = now
		pathtestsToFlush = append(pathtestsToFlush, ptConfigCtx)
		ptConfigCtx.nextRun = ptConfigCtx.nextRun.Add(f.interval)
	}
	return pathtestsToFlush
}

// Add new pathtest
func (f *Store) Add(pathtestToAdd *common.Pathtest) {
	f.logger.Tracef("Add new Pathtest: %+v", pathtestToAdd)

	f.contextsMutex.Lock()
	defer f.contextsMutex.Unlock()

	hash := pathtestToAdd.GetHash()
	pathtestCtx, ok := f.contexts[hash]
	if !ok {
		f.contexts[hash] = newPathtestContext(pathtestToAdd, f.ttl)
		return
	}
	pathtestCtx.runUntil = timeNow().Add(f.ttl)
}

// GetContextsCount returns pathtest contexts count
func (f *Store) GetContextsCount() int {
	f.contextsMutex.Lock()
	defer f.contextsMutex.Unlock()

	return len(f.contexts)
}
