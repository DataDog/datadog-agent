// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pathteststore handle pathtest storage
package pathteststore

import (
	"sync"
	time "time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/time/rate"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
)

const (
	networkPathStoreMetricPrefix = "datadog.network_path.store."
)

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

// Config is the configuration for the PathtestStore
type Config struct {
	// ContextsLimit is the maximum number of contexts to keep in the store
	ContextsLimit int
	// TTL is the duration a Pathtest should run from discovery.
	// If a Pathtest is added again before the TTL expires, the TTL is reset to this duration.
	TTL time.Duration
	// Interval defines how frequently pathtests should run
	Interval time.Duration
	// MaxPerMinute is a "circuit breaker" config that limits pathtests. 0 is unlimited.
	MaxPerMinute int
	// MaxBurstDuration is how long pathtest "budget" can build up in the rate limiter
	MaxBurstDuration time.Duration
}

// Store is used to accumulate aggregated contexts
type Store struct {
	logger       log.Component
	statsdClient ddgostatsd.ClientInterface

	contexts map[uint64]*PathtestContext

	// mutex is needed to protect `contexts` since `Store.add()` and  `pathtestStore.flush()`
	// are called by different routines.
	contextsMutex sync.Mutex

	config Config

	// lastFlushTime is the last time the store was flushed, used by MaxPerMinute limiting
	lastFlushTime time.Time

	// lastContextWarning is the last time a warning was logged about the store being full
	lastContextWarning time.Time

	// rateLimiter is used to limit the number of pathtests that can be flushed per minute
	rateLimiter *rate.Limiter

	// structures needed to ease mocking/testing
	timeNowFn func() time.Time
}

func (f *Store) newPathtestContext(pt *common.Pathtest, runUntilDuration time.Duration) *PathtestContext {
	now := f.timeNowFn()
	return &PathtestContext{
		Pathtest: pt,
		nextRun:  now,
		runUntil: now.Add(runUntilDuration),
	}
}

func (c Config) rateLimiter() *rate.Limiter {
	if c.MaxPerMinute <= 0 {
		return rate.NewLimiter(rate.Inf, 0)
	}

	maxPerMinute := float64(c.MaxPerMinute)
	perSecondRate := rate.Limit(maxPerMinute / 60)

	minutesOfBurst := float64(c.MaxBurstDuration) / float64(time.Minute)
	maxBurst := int(maxPerMinute * minutesOfBurst)

	return rate.NewLimiter(perSecondRate, maxBurst)
}

// NewPathtestStore creates a new Store
func NewPathtestStore(config Config, logger log.Component, statsdClient ddgostatsd.ClientInterface, timeNow func() time.Time) *Store {
	return &Store{
		contexts:      make(map[uint64]*PathtestContext),
		config:        config,
		logger:        logger,
		statsdClient:  statsdClient,
		lastFlushTime: timeNow(),
		rateLimiter:   config.rateLimiter(),
		timeNowFn:     timeNow,
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

	now := f.timeNowFn()
	f.lastFlushTime = now

	var pathtestsToFlush []*PathtestContext
	for key, ptConfigCtx := range f.contexts {
		if ptConfigCtx.runUntil.Before(now) {
			f.logger.Tracef("Delete Pathtest context (key=%d, runUntil=%s, nextRun=%s)", key, ptConfigCtx.runUntil, ptConfigCtx.nextRun)
			// delete ptConfigCtx wrapper if it reaches runUntil
			delete(f.contexts, key)
			if ptConfigCtx.lastFlushTime.IsZero() {
				f.statsdClient.Incr(networkPathStoreMetricPrefix+"pathtest_never_run", []string{}, 1) //nolint:errcheck
			}
			continue
		}
		if ptConfigCtx.nextRun.After(now) || !f.rateLimiter.AllowN(now, 1) {
			continue
		}
		if !ptConfigCtx.lastFlushTime.IsZero() {
			ptConfigCtx.lastFlushInterval = now.Sub(ptConfigCtx.lastFlushTime)
		}
		ptConfigCtx.lastFlushTime = now
		pathtestsToFlush = append(pathtestsToFlush, ptConfigCtx)
		ptConfigCtx.nextRun = ptConfigCtx.nextRun.Add(f.config.Interval)
	}

	f.statsdClient.Gauge(networkPathStoreMetricPrefix+"ratelimiter_tokens", f.rateLimiter.Tokens(), []string{}, 1) //nolint:errcheck

	return pathtestsToFlush
}

// Add new pathtest
func (f *Store) Add(pathtestToAdd *common.Pathtest) {
	f.logger.Tracef("Add new Pathtest: %+v", pathtestToAdd)

	f.contextsMutex.Lock()
	defer f.contextsMutex.Unlock()

	if len(f.contexts) >= f.config.ContextsLimit {
		// only log if it has been 1 minute since the last warning
		if time.Since(f.lastContextWarning) >= time.Minute {
			f.logger.Warnf("Pathteststore is full, maximum set to: %d, dropping pathtest: %+v", f.config.ContextsLimit, pathtestToAdd)
			f.lastContextWarning = time.Now()
		}
		return
	}

	hash := pathtestToAdd.GetHash()
	pathtestCtx, ok := f.contexts[hash]
	if !ok {
		f.contexts[hash] = f.newPathtestContext(pathtestToAdd, f.config.TTL)
		return
	}
	pathtestCtx.runUntil = f.timeNowFn().Add(f.config.TTL)
}

// GetContextsCount returns pathtest contexts count
func (f *Store) GetContextsCount() int {
	f.contextsMutex.Lock()
	defer f.contextsMutex.Unlock()

	return len(f.contexts)
}
