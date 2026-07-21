// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"sync"
	"time"
)

// workloadConfigStore provides spot configuration for workloads.
type workloadConfigStore interface {
	// getConfig returns the workloadSpotConfig for the workload if present.
	getConfig(key objectRef) (workloadSpotConfig, bool)
	// setConfig stores the workloadSpotConfig for key.
	setConfig(key objectRef, cfg workloadSpotConfig)
	// deleteConfig removes the workloadSpotConfig for key.
	deleteConfig(key objectRef)
	// disable disables spot scheduling for workload.
	// If already disabled returns existing timestamp and false,
	// otherwise sets disabledUntil and returns the new timestamp and true.
	disable(key objectRef, now time.Time, until time.Time) (time.Time, bool)
	// countByKind returns the number of workloads grouped by kind.
	countByKind() map[string]int
	// countDisabledByKind returns the number of workloads
	// for which spot-scheduling is disabled grouped by kind.
	countDisabledByKind(now time.Time) map[string]int
}

// spotConfigStore is a thread-safe key-value store of workload spot configs.
// It is optimised for read-heavy access: getConfig is called on every pod
// admission while writes (setConfig, deleteConfig, disable) come from a
// single-threaded workload controller and are rare. A single RWMutex is
// sufficient — concurrent reads do not block each other, and the brief
// write-lock windows are never on the critical path.
type spotConfigStore struct {
	mu      sync.RWMutex
	configs map[objectRef]workloadSpotConfig
}

func newSpotConfigStore() *spotConfigStore {
	return &spotConfigStore{
		configs: make(map[objectRef]workloadSpotConfig),
	}
}

func (s *spotConfigStore) getConfig(key objectRef) (workloadSpotConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configs[key]
	return cfg, ok
}

func (s *spotConfigStore) setConfig(key objectRef, cfg workloadSpotConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[key] = cfg
}

func (s *spotConfigStore) deleteConfig(key objectRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.configs, key)
}

// countByKind returns the number of managed workloads grouped by kind.
func (s *spotConfigStore) countByKind() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byKind := make(map[string]int)
	for key := range s.configs {
		byKind[key.Kind]++
	}
	return byKind
}

// countDisabledByKind returns the number of workloads currently in on-demand fallback mode grouped by kind.
func (s *spotConfigStore) countDisabledByKind(now time.Time) map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byKind := make(map[string]int)
	for key, cfg := range s.configs {
		if cfg.isDisabled(now) {
			byKind[key.Kind]++
		}
	}
	return byKind
}

func (s *spotConfigStore) disable(key objectRef, now time.Time, until time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.configs[key]
	if !ok {
		return time.Time{}, false
	}
	if now.Before(cfg.disabledUntil) {
		return cfg.disabledUntil, false
	}
	cfg.disabledUntil = until
	s.configs[key] = cfg
	return until, true
}
