// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"go.uber.org/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type cacheEntry struct {
	count int
	last  time.Time
}

// Limiter defines a rate limiter which limits tokens to 'numAllowedTokensPerDuration' per 'duration'
type Limiter[K comparable] struct {
	cache                       *simplelru.LRU[K, cacheEntry]
	numAllowedTokensPerDuration int
	duration                    time.Duration

	// stats
	dropped *atomic.Uint64
	allowed *atomic.Uint64
}

// NewLimiter returns a rate limiter that is sized to the configured number of unique tokens, and each unique token is allowed 'numAllowedTokensPerDuration' times per 'duration'.
func NewLimiter[K comparable](numUniqueTokens int, numAllowedTokensPerDuration int, duration time.Duration) (*Limiter[K], error) {
	cache, err := simplelru.NewLRU[K, cacheEntry](numUniqueTokens, nil)
	if err != nil {
		return nil, err
	}

	return &Limiter[K]{
		cache:                       cache,
		numAllowedTokensPerDuration: numAllowedTokensPerDuration,
		duration:                    duration,
		dropped:                     atomic.NewUint64(0),
		allowed:                     atomic.NewUint64(0),
	}, nil
}

// Allow returns whether an entry is allowed or not
func (l *Limiter[K]) Allow(k K) bool {
	if entry, ok := l.cache.Get(k); ok {
		if entry.count < l.numAllowedTokensPerDuration {
			l.Count(k)
		} else if time.Now().Sub(entry.last) >= l.duration {
			// If time elapsed between now and the last cache entry is longer than allowed duration, remove from cache and allow current event
			l.cache.Remove(k)
			l.allowed.Inc()
			return true
		} else {
			l.dropped.Inc()
			return false
		}
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

// Count marks the key as used
func (l *Limiter[K]) Count(k K) {
	cacheEntryToAdd := cacheEntry{
		count: 1,
		last:  time.Now(),
	}

	if val, ok := l.cache.Peek(k); ok {
		incrementedCount := val.count + 1
		cacheEntryToAdd = cacheEntry{
			count: incrementedCount,
			last:  time.Now(),
		}
	}

	l.cache.Add(k, cacheEntryToAdd)
}
