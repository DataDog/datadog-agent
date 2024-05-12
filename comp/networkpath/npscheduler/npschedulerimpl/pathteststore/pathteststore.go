// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package pathteststore

import (
	"sync"
	time "time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler/npschedulerimpl/common"
)

var timeNow = time.Now

// PathtestContext contains Pathtest information and additional flush related data
type PathtestContext struct {
	Pathtest *common.Pathtest

	nextRunTime       time.Time
	runUntilTime      time.Time
	lastFlushTime     time.Time
	lastFlushInterval time.Duration
}

func (p *PathtestContext) LastFlushInterval() time.Duration {
	return p.lastFlushInterval
}

func (p *PathtestContext) SetLastFlushInterval(lastFlushInterval time.Duration) {
	p.lastFlushInterval = lastFlushInterval
}

// PathtestStore is used to accumulate aggregated pathtestContexts
type PathtestStore struct {
	logger log.Component

	pathtestContexts map[uint64]*PathtestContext
	// mutex is needed to protect `pathtestContexts` since `PathtestStore.add()` and  `pathtestStore.flush()`
	// are called by different routines.
	pathtestConfigsMutex sync.Mutex

	// flushInterval defines how frequently we check for paths to be run
	// TODO: NOT NEEDED, FLUSH HAPPENS AT NPSCHEDULER?
	flushInterval time.Duration

	// pathtestInterval defines how frequently pathtests should run
	pathtestInterval time.Duration

	// pathtestTTL is the duration a Pathtest should run from discovery.
	// If a Pathtest is added again before the TTL expires, the TTL is reset to this duration.
	pathtestTTL time.Duration
}

func newPathtestContext(pt *common.Pathtest, runUntilDuration time.Duration) *PathtestContext {
	now := timeNow()
	return &PathtestContext{
		Pathtest:     pt,
		nextRunTime:  now,
		runUntilTime: now.Add(runUntilDuration),
	}
}

func NewPathtestStore(flushInterval time.Duration, pathtestTTL time.Duration, pathtestInterval time.Duration, logger log.Component) *PathtestStore {
	return &PathtestStore{
		pathtestContexts: make(map[uint64]*PathtestContext),
		flushInterval:    flushInterval,
		pathtestTTL:      pathtestTTL,
		pathtestInterval: pathtestInterval,
		logger:           logger,
	}
}

// Flush will flush specific Pathtest context (distinct hash) if nextRunTime is reached
// once a Pathtest context is flushed nextRunTime will be updated to the next flush time
//
// pathtestTTL:
// pathtestTTL defines the duration we should keep a specific PathtestContext in `PathtestStore.pathtestContexts`
// after `lastSuccessfulFlush`. // Flow context in `PathtestStore.pathtestContexts` map will be deleted if `pathtestTTL`
// is reached to avoid keeping Pathtest context that are not seen anymore.
// We need to keep PathtestContext (contains `nextRunTime` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flushInterval`).
// Example, after a flush, PathtestContext will have a new nextRunTime, that will be the next flush time for new pathtestContexts being added.
func (f *PathtestStore) Flush() []*PathtestContext {
	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	f.logger.Tracef("f.pathtestContexts: %+v", f.pathtestContexts)
	// DEBUG STATEMENTS
	for _, ptConf := range f.pathtestContexts {
		if ptConf.Pathtest != nil {
			f.logger.Tracef("in-mem ptConf %s:%d", ptConf.Pathtest.Hostname, ptConf.Pathtest.Port)
		}
	}

	var pathtestsToFlush []*PathtestContext
	for key, ptConfigCtx := range f.pathtestContexts {
		now := timeNow()

		if ptConfigCtx.runUntilTime.Before(now) {
			f.logger.Tracef("Delete Pathtest context (key=%d, runUntilTime=%s, nextRunTime=%s)", key, ptConfigCtx.runUntilTime.String(), ptConfigCtx.nextRunTime.String())
			// delete ptConfigCtx wrapper if it reaches runUntilTime
			delete(f.pathtestContexts, key)
			continue
		}
		if ptConfigCtx.nextRunTime.After(now) {
			continue
		}
		// TODO: test lastFlushTime and lastFlushInterval
		if !ptConfigCtx.lastFlushTime.IsZero() {
			ptConfigCtx.lastFlushInterval = now.Sub(ptConfigCtx.lastFlushTime)
		}
		ptConfigCtx.lastFlushTime = now
		pathtestsToFlush = append(pathtestsToFlush, ptConfigCtx)
		// TODO: increment nextRunTime to a time after current time
		//       in case flush() is not fast enough, it won't accumulate excessively
		ptConfigCtx.nextRunTime = ptConfigCtx.nextRunTime.Add(f.pathtestInterval)
		f.pathtestContexts[key] = ptConfigCtx
	}
	return pathtestsToFlush
}

// Add TODO
func (f *PathtestStore) Add(pathtestToAdd *common.Pathtest) {
	f.logger.Tracef("Add new Pathtest: %+v", pathtestToAdd)

	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	hash := pathtestToAdd.GetHash()
	pathtestCtx, ok := f.pathtestContexts[hash]
	if !ok {
		f.pathtestContexts[hash] = newPathtestContext(pathtestToAdd, f.pathtestTTL)
		return
	}
	pathtestCtx.runUntilTime = timeNow().Add(f.pathtestTTL)
	f.pathtestContexts[hash] = pathtestCtx
}

// GetPathtestContextCount TODO
func (f *PathtestStore) GetPathtestContextCount() int {
	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	return len(f.pathtestContexts)
}
