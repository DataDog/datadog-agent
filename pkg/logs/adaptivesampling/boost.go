// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package adaptivesampling contains shared coordination primitives for logs
// adaptive sampling.
package adaptivesampling

import (
	"sync"
	"time"
)

// SamplingBoost asks the logs adaptive sampler to temporarily raise allowance
// for a container and, optionally, a specific tokenized log pattern.
type SamplingBoost struct {
	ID              uint64
	ContainerID     string
	PatternHash     string
	ExpiresAt       time.Time
	RateMultiplier  float64
	BurstMultiplier float64
	CreditGrant     float64
}

type boostKey struct {
	containerID string
	patternHash string
}

// SamplingBoostStore stores short-lived sampler boosts.
type SamplingBoostStore struct {
	mu     sync.Mutex
	nextID uint64
	boosts map[boostKey]SamplingBoost
}

// NewSamplingBoostStore creates an empty boost store.
func NewSamplingBoostStore() *SamplingBoostStore {
	return &SamplingBoostStore{
		boosts: make(map[boostKey]SamplingBoost),
	}
}

var defaultSamplingBoostStore = NewSamplingBoostStore()

// DefaultSamplingBoostStore returns the process-wide boost store used by the
// observer and logs sampler POC integration.
func DefaultSamplingBoostStore() *SamplingBoostStore {
	return defaultSamplingBoostStore
}

// ResetDefaultSamplingBoostStoreForTest clears the process-wide boost store.
func ResetDefaultSamplingBoostStoreForTest() {
	defaultSamplingBoostStore.Reset()
}

// Set stores a boost and assigns a monotonically increasing ID.
func (s *SamplingBoostStore) Set(boost SamplingBoost) SamplingBoost {
	if s == nil || boost.ContainerID == "" {
		return SamplingBoost{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	boost.ID = s.nextID
	if boost.RateMultiplier <= 0 {
		boost.RateMultiplier = 1
	}
	if boost.BurstMultiplier <= 0 {
		boost.BurstMultiplier = 1
	}
	s.boosts[boost.key()] = boost
	return boost
}

// Lookup returns the best boost for containerID/patternHash at now. Exact
// container+pattern boosts take precedence over container-wide boosts.
func (s *SamplingBoostStore) Lookup(containerID, patternHash string, now time.Time) (SamplingBoost, bool) {
	boost, ok, _ := s.LookupWithActiveCount(containerID, patternHash, now)
	return boost, ok
}

// LookupWithActiveCount returns the best boost and the number of active boosts
// retained after expiring stale entries. The count is useful for bounded POC
// diagnostics when a sampler misses despite active boosts existing.
func (s *SamplingBoostStore) LookupWithActiveCount(containerID, patternHash string, now time.Time) (SamplingBoost, bool, int) {
	if s == nil || containerID == "" {
		return SamplingBoost{}, false, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	activeCount := s.expireLocked(now)
	if patternHash != "" {
		if boost, ok := s.lookupLocked(boostKey{containerID: containerID, patternHash: patternHash}, now); ok {
			return boost, true, activeCount
		}
	}
	boost, ok := s.lookupLocked(boostKey{containerID: containerID}, now)
	return boost, ok, activeCount
}

func (s *SamplingBoostStore) lookupLocked(key boostKey, now time.Time) (SamplingBoost, bool) {
	boost, ok := s.boosts[key]
	if !ok {
		return SamplingBoost{}, false
	}
	if !boost.ExpiresAt.IsZero() && !boost.ExpiresAt.After(now) {
		delete(s.boosts, key)
		return SamplingBoost{}, false
	}
	return boost, true
}

func (s *SamplingBoostStore) expireLocked(now time.Time) int {
	activeCount := 0
	for key, boost := range s.boosts {
		if !boost.ExpiresAt.IsZero() && !boost.ExpiresAt.After(now) {
			delete(s.boosts, key)
			continue
		}
		activeCount++
	}
	return activeCount
}

// Reset clears all boosts.
func (s *SamplingBoostStore) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID = 0
	s.boosts = make(map[boostKey]SamplingBoost)
}

func (b SamplingBoost) key() boostKey {
	return boostKey{
		containerID: b.ContainerID,
		patternHash: b.PatternHash,
	}
}
