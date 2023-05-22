// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// Limiter defines generic rate limiter
type Limiter[K comparable] struct {
	cache  *simplelru.LRU[K, time.Time]
	period time.Duration
}

// NewLimiter returns a rate limiter
func NewLimiter[K comparable](size int, period time.Duration) (*Limiter[K], error) {
	cache, err := simplelru.NewLRU[K, time.Time](size, nil)
	if err != nil {
		return nil, err
	}

	return &Limiter[K]{
		cache:  cache,
		period: period,
	}, nil
}

// IsAllowed returns whether an entry is allowed or not
func (l *Limiter[K]) IsAllowed(k K) bool {
	now := time.Now()
	if ts, ok := l.cache.Get(k); ok {
		if now.After(ts) {
			l.cache.Remove(k)
		} else {
			return false
		}
	}

	return true
}

// Count marks the key as used
func (l *Limiter[K]) Count(k K) {
	l.cache.Add(k, time.Now().Add(l.period))
}
