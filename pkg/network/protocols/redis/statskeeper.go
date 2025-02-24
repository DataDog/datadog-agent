// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StatsKeeper is a struct to hold the records for the redis protocol
type StatsKeeper struct {
	stats      map[Key]*RequestStat
	statsMutex sync.RWMutex
	maxEntries int
}

// NewStatsKeeper creates a new Redis StatsKeeper
func NewStatsKeeper(c *config.Config) *StatsKeeper {
	statsKeeper := &StatsKeeper{
		maxEntries: c.MaxRedisStatsBuffered,
	}

	statsKeeper.resetNoLock()
	return statsKeeper
}

// Process processes the redis transaction
func (s *StatsKeeper) Process(event *EventWrapper) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	key := Key{
		Command:       event.CommandType(),
		KeyName:       event.KeyName(),
		ConnectionKey: event.ConnTuple(),
		Truncated:     event.Tx.Truncated,
	}

	requestStats, ok := s.stats[key]
	if !ok {
		if len(s.stats) >= s.maxEntries {
			return
		}
		requestStats = new(RequestStat)
		s.stats[key] = requestStats
	}
	requestStats.StaticTags = uint64(event.Tx.Tags)
	requestStats.Count++
	if requestStats.Count == 1 {
		requestStats.FirstLatencySample = event.RequestLatency()
		return
	}
	if requestStats.Latencies == nil {
		if err := requestStats.initSketch(); err != nil {
			log.Warnf("could not add request latency to ddsketch: %v", err)
			return
		}
		if err := requestStats.Latencies.Add(requestStats.FirstLatencySample); err != nil {
			return
		}
	}
	if err := requestStats.Latencies.Add(event.RequestLatency()); err != nil {
		log.Debugf("could not add request latency to ddsketch: %v", err)
	}
}

// GetAndResetAllStats returns all the records and resets the statskeeper
func (s *StatsKeeper) GetAndResetAllStats() map[Key]*RequestStat {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()

	ret := s.stats
	s.resetNoLock()
	return ret
}

func (s *StatsKeeper) resetNoLock() {
	s.stats = make(map[Key]*RequestStat)
}
