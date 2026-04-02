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
	getConfig(key workload) (workloadSpotConfig, bool)
	// setConfig stores the workloadSpotConfig for key.
	setConfig(key workload, cfg workloadSpotConfig)
	// deleteConfig removes the workloadSpotConfig for key.
	deleteConfig(key workload)
	// disable disables spot scheduling for workload.
	// If already disabled returns existing timestamp and false,
	// otherwise sets disabledUntil and returns the new timestamp and true.
	disable(key workload, now time.Time, until time.Time) (time.Time, bool)
}

// spotConfigStore is a thread-safe key-value store of workload spot configs.
type spotConfigStore struct {
	mu      sync.RWMutex
	configs map[workload]workloadSpotConfig
}

func newSpotConfigStore() *spotConfigStore {
	return &spotConfigStore{
		configs: make(map[workload]workloadSpotConfig),
	}
}

func (s *spotConfigStore) getConfig(key workload) (workloadSpotConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configs[key]
	return cfg, ok
}

func (s *spotConfigStore) setConfig(key workload, cfg workloadSpotConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[key] = cfg
}

func (s *spotConfigStore) deleteConfig(key workload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.configs, key)
}

func (s *spotConfigStore) disable(key workload, now time.Time, until time.Time) (time.Time, bool) {
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
