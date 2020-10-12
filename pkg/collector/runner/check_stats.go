// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package runner

import (
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// How long is the first series of check runs we want to log
	firstRunSeries uint64 = 5
)

var checkStats *runnerCheckStats

func init() {
	checkStats = &runnerCheckStats{
		Stats: make(map[string]map[check.ID]*check.Stats),
	}
}

// runnerCheckStats holds the stats from the running checks
type runnerCheckStats struct {
	Stats map[string]map[check.ID]*check.Stats
	M     sync.RWMutex
}

// AddWorkStats updates the stats of a given check, should be called after every check run
// The runner.work() method calls this function to collect regular-corecheck stats
// Long-running corechecks should explicitly call this function to register and update their stats
func AddWorkStats(c check.Check, execTime time.Duration, err error, warnings []error, mStats map[string]int64) {
	var s *check.Stats
	var found bool

	checkStats.M.Lock()
	log.Tracef("Add stats for %s", string(c.ID()))
	stats, found := checkStats.Stats[c.String()]
	if !found {
		stats = make(map[check.ID]*check.Stats)
		checkStats.Stats[c.String()] = stats
	}
	s, found = stats[c.ID()]
	if !found {
		s = check.NewStats(c)
		stats[c.ID()] = s
	}
	checkStats.M.Unlock()

	s.Add(execTime, err, warnings, mStats)
}

// GetCheckStats returns the check stats map
func GetCheckStats() map[string]map[check.ID]*check.Stats {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	return checkStats.Stats
}

// RemoveCheckStats removes a check from the check stats map
func RemoveCheckStats(checkID check.ID) {
	checkStats.M.Lock()
	defer checkStats.M.Unlock()
	log.Debugf("Remove stats for %s", string(checkID))

	checkName := strings.Split(string(checkID), ":")[0]
	stats, found := checkStats.Stats[checkName]
	if found {
		delete(stats, checkID)
		if len(stats) == 0 {
			delete(checkStats.Stats, checkName)
		}
	}
}

func shouldLog(id check.ID) (doLog bool, lastLog bool) {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	var nameFound, idFound bool
	var s *check.Stats

	loggingFrequency := uint64(config.Datadog.GetInt64("logging_frequency"))
	name := strings.Split(string(id), ":")[0]

	stats, nameFound := checkStats.Stats[name]
	if nameFound {
		s, idFound = stats[id]
	}
	// this is the first time we see the check, log it
	if !idFound {
		doLog = true
		lastLog = false
		return
	}

	// we log the first firstRunSeries times, then every loggingFrequency times
	doLog = s.TotalRuns <= firstRunSeries || s.TotalRuns%loggingFrequency == 0
	// we print a special message when we change logging frequency
	lastLog = s.TotalRuns == firstRunSeries
	return
}

func expCheckStats() interface{} {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	return checkStats.Stats
}
