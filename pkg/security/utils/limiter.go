// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"time"

	"go.uber.org/atomic"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type cacheEntry struct {
	count         int
	timeFirstSeen time.Time // marks the beginning of the time window during which a limited number of tokens are allowed
}

// LimiterStat return stats
type LimiterStat struct {
	Dropped uint64
	Allowed uint64
	Tags    []string
}

// Limiter defines a rate limiter which limits tokens to 'numAllowedTokensPerPeriod' per 'period'
type Limiter[K comparable] struct {
	cache                     *simplelru.LRU[K, *cacheEntry]
	numAllowedTokensPerPeriod int
	period                    time.Duration

	// stats
	dropped *atomic.Uint64
	allowed *atomic.Uint64
}

// NewLimiter returns a rate limiter that is sized to the configured number of unique tokens, and each unique token is allowed 'numAllowedTokensPerPeriod' times per 'period'.
func NewLimiter[K comparable](numUniqueTokens int, numAllowedTokensPerPeriod int, period time.Duration) (*Limiter[K], error) {
	cache, err := simplelru.NewLRU[K, *cacheEntry](numUniqueTokens, nil)
	if err != nil {
		return nil, err
	}

	return &Limiter[K]{
		cache:                     cache,
		numAllowedTokensPerPeriod: numAllowedTokensPerPeriod,
		period:                    period,
		dropped:                   atomic.NewUint64(0),
		allowed:                   atomic.NewUint64(0),
	}, nil
}

// Allow returns whether an entry is allowed or not
func (l *Limiter[K]) Allow(k K) bool {
	if entry, ok := l.cache.Get(k); ok {
		if time.Since(entry.timeFirstSeen) >= l.period {
			// If time elapsed between now and the first cache entry is longer than allowed period, reset the count and allow
			l.init(k)
		} else if entry.count < l.numAllowedTokensPerPeriod {
			l.Count(k)
		} else {
			l.dropped.Inc()
			return false
		}
	} else {
		l.init(k)
	}

	l.allowed.Inc()
	return true
}

// SwapStats returns the dropped and allowed stats, and zeros the stats
func (l *Limiter[K]) SwapStats() []LimiterStat {
	return []LimiterStat{
		{
			Dropped: l.dropped.Swap(0),
			Allowed: l.allowed.Swap(0),
		},
	}
}

// init marks the key as used with a count of 1
func (l *Limiter[K]) init(k K) {
	entry := &cacheEntry{
		count:         1,
		timeFirstSeen: time.Now(),
	}
	l.cache.Add(k, entry)
}

// Count marks the key as used and increments the count
func (l *Limiter[K]) Count(k K) {
	// use get to mark it as used so that it won't be evicted
	if entry, ok := l.cache.Get(k); ok {
		entry.count++
	} else {
		l.init(k)
	}
}
