// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package expvars

import (
	"expvar"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Top-level expvar (the convention for them is that they are lowercase)
	runnerExpvarKey = "runner"

	// Nested keys
	checksExpvarKey        = "Checks"
	errorsExpvarKey        = "Errors"
	runningChecksExpvarKey = "RunningChecks"
	runsExpvarKey          = "Runs"
	runningExpvarKey       = "Running"
	warningsExpvarKey      = "Warnings"
)

var (
	runnerStats        *expvar.Map
	runningChecksStats *expvar.Map
	checkStats         *expCheckStats
)

// expCheckStats holds the stats from the running checks
type expCheckStats struct {
	stats     map[string]map[checkid.ID]*checkstats.Stats
	statsLock sync.RWMutex
}

func init() {
	runningChecksStats = &expvar.Map{}

	runnerStats = expvar.NewMap(runnerExpvarKey)
	runnerStats.Set(checksExpvarKey, expvar.Func(expCheckStatsFunc))
	runnerStats.Set(runningExpvarKey, runningChecksStats)

	newWorkersExpvar(runnerStats)

	checkStats = &expCheckStats{
		stats: make(map[string]map[checkid.ID]*checkstats.Stats),
	}
}

// Helpers

func expCheckStatsFunc() interface{} {
	return GetCheckStats()
}

// Reset clears all stats collected so far (useful in testing)
func Reset() {
	log.Warnf("Resetting all check stats")

	checkStats.statsLock.Lock()
	defer checkStats.statsLock.Unlock()

	// Clear checks stats
	for key := range checkStats.stats {
		delete(checkStats.stats, key)
	}

	// Clear running checks map
	runningChecksStats.Init()

	// Clear top-level expvars on the runner
	for _, key := range []string{
		errorsExpvarKey,
		runsExpvarKey,
		runningChecksExpvarKey,
		warningsExpvarKey,
	} {
		runnerStats.Delete(key)
	}

	resetWorkersExpvar(runnerStats)
}

// Functions relating to check run stats (`checkStats`)

// GetCheckStats returns the check stats map
func GetCheckStats() map[string]map[checkid.ID]*checkstats.Stats {
	checkStats.statsLock.RLock()
	defer checkStats.statsLock.RUnlock()

	// Because the returned maps will be used after the lock is released, and
	// thus when they might be further modified, we must clone them here.  The
	// map values (`stats.Stats`) are threadsafe and need not be cloned.

	cloned := make(map[string]map[checkid.ID]*checkstats.Stats)
	for k, v := range checkStats.stats {
		innerCloned := make(map[checkid.ID]*checkstats.Stats)
		for innerK, innerV := range v {
			innerCloned[innerK] = innerV
		}
		cloned[k] = innerCloned
	}

	return cloned
}

// AddCheckStats adds runtime stats to the check's expvars
func AddCheckStats(
	c check.Check,
	execTime time.Duration,
	err error,
	warnings []error,
	mStats checkstats.SenderStats,
) {

	var s *checkstats.Stats

	checkStats.statsLock.Lock()
	defer checkStats.statsLock.Unlock()

	log.Tracef("Adding stats for %s", string(c.ID()))

	checkName := checkid.IDToCheckName(c.ID())
	stats, found := checkStats.stats[checkName]
	if !found {
		stats = make(map[checkid.ID]*checkstats.Stats)
		checkStats.stats[checkName] = stats
	}

	s, found = stats[c.ID()]
	if !found {
		s = checkstats.NewStats(c)
		stats[c.ID()] = s
	}

	s.Add(execTime, err, warnings, mStats)
}

// RemoveCheckStats removes a check from the check stats map
func RemoveCheckStats(checkID checkid.ID) {
	checkStats.statsLock.Lock()
	defer checkStats.statsLock.Unlock()

	log.Debugf("Removing stats for %s", string(checkID))

	checkName := checkid.IDToCheckName(checkID)
	stats, found := checkStats.stats[checkName]

	if !found {
		log.Warnf("Stats for check %s not found", string(checkID))
		return
	}

	delete(stats, checkID)

	if len(stats) == 0 {
		delete(checkStats.stats, checkName)
	}
}

// CheckStats returns the check stats of a check, if they can be found
func CheckStats(id checkid.ID) (*checkstats.Stats, bool) {
	checkStats.statsLock.RLock()
	defer checkStats.statsLock.RUnlock()

	checkName := checkid.IDToCheckName(id)
	stats, nameFound := checkStats.stats[checkName]

	if !nameFound {
		return nil, false
	}

	check, checkFound := stats[id]
	if !checkFound {
		return nil, false
	}

	return check, true
}

// Functions relating to running checks state map (`runningChecksStats`)

// SetRunningStats sets the start time of a running check
func SetRunningStats(id checkid.ID, t time.Time) {
	runningChecksStats.Set(string(id), timestamp(t))
}

// GetRunningStats gets the start time of a running check
func GetRunningStats(id checkid.ID) time.Time {
	startTimeExpvar := runningChecksStats.Get(string(id))
	if startTimeExpvar == nil {
		// "Zero" time
		return time.Time{}
	}
	return time.Time(startTimeExpvar.(timestamp))
}

// DeleteRunningStats clears the start time of a check when it's complete
func DeleteRunningStats(id checkid.ID) {
	runningChecksStats.Delete(string(id))
}

// AddRunningCheckCount is used to increment and decrement the 'RunningChecks' expvar
func AddRunningCheckCount(amount int) {
	runnerStats.Add(runningChecksExpvarKey, int64(amount))
}

// GetRunningCheckCount is used to get the value of 'RunningChecks' expvar
func GetRunningCheckCount() int64 {
	count := runnerStats.Get(runningChecksExpvarKey)
	if count == nil {
		return 0
	}

	return count.(*expvar.Int).Value()
}

// AddRunsCount is used to increment and decrement the 'Runs' expvar
func AddRunsCount(amount int) {
	runnerStats.Add(runsExpvarKey, int64(amount))
}

// GetRunsCount is used to get the value of 'Runs' expvar
func GetRunsCount() int64 {
	count := runnerStats.Get(runsExpvarKey)
	if count == nil {
		return 0
	}
	return count.(*expvar.Int).Value()
}

// AddWarningsCount is used to increment the 'Warnings' expvar
func AddWarningsCount(amount int) {
	runnerStats.Add(warningsExpvarKey, int64(amount))
}

// GetWarningsCount is used to get the value of 'Warnings' expvar
func GetWarningsCount() int64 {
	count := runnerStats.Get(warningsExpvarKey)
	if count == nil {
		return 0
	}
	return count.(*expvar.Int).Value()
}

// AddErrorsCount is used to increment the 'Errors' expvar
func AddErrorsCount(amount int) {
	runnerStats.Add(errorsExpvarKey, int64(amount))
}

// GetErrorsCount is used to get the value of 'Errors' expvar
func GetErrorsCount() int64 {
	count := runnerStats.Get(errorsExpvarKey)
	if count == nil {
		return 0
	}
	return count.(*expvar.Int).Value()
}
