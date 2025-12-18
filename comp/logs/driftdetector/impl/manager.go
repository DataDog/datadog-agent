// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package impl

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// driftDetectorManager manages multiple drift detector instances, one per log source
type driftDetectorManager struct {
	config  common.Config
	enabled bool

	mu         sync.RWMutex
	detectors  map[string]*pipeline.Pipeline // sourceKey -> pipeline
	lastAccess map[string]time.Time          // sourceKey -> last access time

	// Cleanup
	cleanupInterval time.Duration
	maxIdleTime     time.Duration
	stopCleanup     chan struct{}
}

// newDriftDetectorManager creates a new manager for per-source drift detectors
func newDriftDetectorManager(config common.Config) *driftDetectorManager {
	return &driftDetectorManager{
		config:          config,
		enabled:         config.Embedding.Enabled,
		detectors:       make(map[string]*pipeline.Pipeline),
		lastAccess:      make(map[string]time.Time),
		cleanupInterval: config.Manager.CleanupInterval,
		maxIdleTime:     config.Manager.MaxIdleTime,
		stopCleanup:     make(chan struct{}),
	}
}

// Start starts the manager and cleanup routine
func (m *driftDetectorManager) Start() error {
	if !m.enabled {
		log.Info("Drift detector manager is disabled")
		return nil
	}

	log.Info("Starting drift detector manager")
	go m.cleanupRoutine()
	return nil
}

// Stop stops all drift detectors and the manager
func (m *driftDetectorManager) Stop() {
	if !m.enabled {
		return
	}

	log.Info("Stopping drift detector manager")
	close(m.stopCleanup)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop all active detectors
	for sourceKey, detector := range m.detectors {
		log.Infof("Stopping drift detector for source: %s", sourceKey)
		detector.Stop()
	}

	m.detectors = make(map[string]*pipeline.Pipeline)
	m.lastAccess = make(map[string]time.Time)
}

// ProcessLog processes a log from a specific source
func (m *driftDetectorManager) ProcessLog(sourceKey string, timestamp time.Time, content string) {
	if !m.enabled {
		return
	}

	// Get or create detector for this source
	detector := m.getOrCreateDetector(sourceKey)
	if detector == nil {
		return
	}

	// Update last access time
	m.mu.Lock()
	m.lastAccess[sourceKey] = time.Now()
	m.mu.Unlock()

	// Process the log
	detector.ProcessLog(timestamp, content)
}

// getOrCreateDetector gets an existing detector or creates a new one for the source
func (m *driftDetectorManager) getOrCreateDetector(sourceKey string) *pipeline.Pipeline {
	// Try to get existing detector (fast path with read lock)
	m.mu.RLock()
	detector, exists := m.detectors[sourceKey]
	m.mu.RUnlock()

	if exists {
		return detector
	}

	// Create new detector (slow path with write lock)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check in case another goroutine created it
	detector, exists = m.detectors[sourceKey]
	if exists {
		return detector
	}

	// Create new pipeline for this source
	log.Infof("Creating drift detector for source: %s", sourceKey)
	detector = pipeline.New(m.config)

	// Start the pipeline
	if err := detector.Start(); err != nil {
		log.Errorf("Failed to start drift detector for source %s: %v", sourceKey, err)
		return nil
	}

	// Store detector
	m.detectors[sourceKey] = detector
	m.lastAccess[sourceKey] = time.Now()

	log.Infof("Drift detector created for source: %s (total detectors: %d)", sourceKey, len(m.detectors))
	return detector
}

// removeDetector removes and stops a detector for a source
func (m *driftDetectorManager) removeDetector(sourceKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	detector, exists := m.detectors[sourceKey]
	if !exists {
		return
	}

	log.Infof("Removing idle drift detector for source: %s", sourceKey)
	detector.Stop()
	delete(m.detectors, sourceKey)
	delete(m.lastAccess, sourceKey)

	log.Infof("Drift detector removed (remaining detectors: %d)", len(m.detectors))
}

// cleanupRoutine periodically removes idle detectors to free resources
func (m *driftDetectorManager) cleanupRoutine() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCleanup:
			return
		case <-ticker.C:
			m.cleanupIdleDetectors()
		}
	}
}

// cleanupIdleDetectors removes detectors that haven't been used recently
func (m *driftDetectorManager) cleanupIdleDetectors() {
	m.mu.RLock()
	now := time.Now()
	toRemove := []string{}

	for sourceKey, lastAccess := range m.lastAccess {
		if now.Sub(lastAccess) > m.maxIdleTime {
			toRemove = append(toRemove, sourceKey)
		}
	}
	m.mu.RUnlock()

	// Remove idle detectors
	for _, sourceKey := range toRemove {
		m.removeDetector(sourceKey)
	}

	if len(toRemove) > 0 {
		log.Infof("Cleaned up %d idle drift detectors", len(toRemove))
	}
}

// GetStats returns statistics about active detectors
func (m *driftDetectorManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"enabled":          m.enabled,
		"active_detectors": len(m.detectors),
		"sources":          m.getSourceKeys(),
	}
}

func (m *driftDetectorManager) getSourceKeys() []string {
	keys := make([]string, 0, len(m.detectors))
	for k := range m.detectors {
		keys = append(keys, k)
	}
	return keys
}

// IsEnabled returns whether the manager is enabled
func (m *driftDetectorManager) IsEnabled() bool {
	return m.enabled
}
