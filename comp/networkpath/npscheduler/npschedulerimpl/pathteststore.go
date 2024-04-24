package npschedulerimpl

import (
	"sync"
	time "time"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

var timeNow = time.Now

// pathtestContext contains pathtest information and additional flush related data
type pathtestContext struct {
	pathtest     *pathtest
	nextRunTime  time.Time
	runUntilTime time.Time
}

// pathtestStore is used to accumulate aggregated pathtestContexts
type pathtestStore struct {
	logger log.Component

	pathtestContexts map[uint64]pathtestContext
	// mutex is needed to protect `pathtestContexts` since `pathtestStore.add()` and  `pathtestStore.flush()`
	// are called by different routines.
	pathtestConfigsMutex sync.Mutex

	flushInterval            time.Duration
	pathtestRunInterval      time.Duration
	pathtestRunUntilDuration time.Duration
}

func newPathtestContext(pt *pathtest, runUntilDuration time.Duration) pathtestContext {
	now := timeNow()
	return pathtestContext{
		pathtest:     pt,
		nextRunTime:  now,
		runUntilTime: now.Add(runUntilDuration),
	}
}

func newPathtestStore(flushInterval time.Duration, pathtestRunUntilDuration time.Duration, pathtestRunInterval time.Duration, logger log.Component) *pathtestStore {
	return &pathtestStore{
		pathtestContexts:         make(map[uint64]pathtestContext),
		flushInterval:            flushInterval,
		pathtestRunUntilDuration: pathtestRunUntilDuration,
		pathtestRunInterval:      pathtestRunInterval,
		logger:                   logger,
	}
}

// flush will flush specific pathtest context (distinct hash) if nextRunTime is reached
// once a pathtest context is flushed nextRunTime will be updated to the next flush time
//
// pathtestRunUntilDuration:
// pathtestRunUntilDuration defines the duration we should keep a specific pathtestContext in `pathtestStore.pathtestContexts`
// after `lastSuccessfulFlush`. // Flow context in `pathtestStore.pathtestContexts` map will be deleted if `pathtestRunUntilDuration`
// is reached to avoid keeping pathtest context that are not seen anymore.
// We need to keep pathtestContext (contains `nextRunTime` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flushInterval`).
// Example, after a flush, pathtestContext will have a new nextRunTime, that will be the next flush time for new pathtestContexts being added.
func (f *pathtestStore) flush() []*pathtest {
	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	f.logger.Tracef("f.pathtestContexts: %+v", f.pathtestContexts)
	// DEBUG STATEMENTS
	for _, ptConf := range f.pathtestContexts {
		if ptConf.pathtest != nil {
			f.logger.Tracef("in-mem ptConf %s:%d", ptConf.pathtest.hostname, ptConf.pathtest.port)
		}
	}

	var pathtestsToFlush []*pathtest
	for key, ptConfigCtx := range f.pathtestContexts {
		now := timeNow()

		if ptConfigCtx.runUntilTime.Before(now) {
			f.logger.Tracef("Delete pathtest context (key=%d, runUntilTime=%s, nextRunTime=%s)", key, ptConfigCtx.runUntilTime.String(), ptConfigCtx.nextRunTime.String())
			// delete ptConfigCtx wrapper if it reaches runUntilTime
			delete(f.pathtestContexts, key)
			continue
		}
		if ptConfigCtx.nextRunTime.After(now) {
			continue
		}
		pathtestsToFlush = append(pathtestsToFlush, ptConfigCtx.pathtest)
		// TODO: increment nextRunTime to a time after current time
		//       in case flush() is not fast enough, it won't accumulate excessively
		ptConfigCtx.nextRunTime = ptConfigCtx.nextRunTime.Add(f.pathtestRunInterval)
		f.pathtestContexts[key] = ptConfigCtx
	}
	return pathtestsToFlush
}

func (f *pathtestStore) add(pathtestToAdd *pathtest) {
	f.logger.Tracef("Add new pathtest: %+v", pathtestToAdd)

	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	hash := pathtestToAdd.getHash()
	pathtestCtx, ok := f.pathtestContexts[hash]
	if !ok {
		f.pathtestContexts[hash] = newPathtestContext(pathtestToAdd, f.pathtestRunUntilDuration)
		return
	}
	pathtestCtx.runUntilTime = timeNow().Add(f.pathtestRunUntilDuration)
	f.pathtestContexts[hash] = pathtestCtx
}

func (f *pathtestStore) getPathtestContextCount() int {
	f.pathtestConfigsMutex.Lock()
	defer f.pathtestConfigsMutex.Unlock()

	return len(f.pathtestContexts)
}
