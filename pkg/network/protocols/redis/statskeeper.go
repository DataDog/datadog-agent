// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
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
func (s *StatsKeeper) Process(*EbpfEvent) {
	// TODO: process logic
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
