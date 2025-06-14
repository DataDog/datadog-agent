// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// StatsKeeper is a struct to hold the records for the redis protocol
type StatsKeeper struct {
	stats      map[Key]*RequestStats
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
	if event.CommandType() >= maxCommand {
		return
	}

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
		requestStats = NewRequestStats()
		s.stats[key] = requestStats
	}
	count := 1 // We process one event at a time
	requestStats.AddRequest(event.Tx.Is_error, count, uint64(event.Tx.Tags), event.RequestLatency())
}

// GetAndResetAllStats returns all the records and resets the statskeeper
func (s *StatsKeeper) GetAndResetAllStats() map[Key]*RequestStats {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()

	ret := s.stats
	s.resetNoLock()
	return ret
}

func (s *StatsKeeper) resetNoLock() {
	s.stats = make(map[Key]*RequestStats)
}
