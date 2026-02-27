// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ActionTTL is how long actions are valid for execution
	ActionTTL = 1 * time.Minute
	// RecordRetentionTTL is how long action records are kept in memory (24 hours)
	// This allows for inspection and debugging of executed actions
	RecordRetentionTTL = 24 * time.Hour
	// CleanupInterval is how often we clean up expired action records from memory
	CleanupInterval = 30 * time.Second
)

// ActionKey uniquely identifies an action by its metadata ID and version
type ActionKey struct {
	ID      string
	Version uint64
}

// String returns a string representation of the ActionKey
func (ak ActionKey) String() string {
	return fmt.Sprintf("%s:v%d", ak.ID, ak.Version)
}

// ActionRecord stores information about a processed action
type ActionRecord struct {
	Key             ActionKey
	Status          string // "success", "failed", "skipped", "expired"
	Message         string
	ExecutedAt      int64 // Unix timestamp when action was executed
	ReceivedAt      int64 // Unix timestamp when action was received by agent
	ActionCreatedAt int64 // Unix timestamp from action.timestamp field
}

// ActionStore tracks processed actions in-memory to prevent duplicate execution
// Actions older than ActionTTL are automatically expired
type ActionStore struct {
	executed map[string]ActionRecord
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewActionStore creates a new ActionStore and starts the cleanup goroutine
func NewActionStore(ctx context.Context) *ActionStore {
	store := &ActionStore{
		executed: make(map[string]ActionRecord),
		stopCh:   make(chan struct{}),
	}

	// Start background cleanup goroutine
	go store.cleanupLoop(ctx)

	log.Infof("Created in-memory action store with action TTL=%v, record retention=%v, cleanup interval=%v",
		ActionTTL, RecordRetentionTTL, CleanupInterval)
	return store
}

// IsExpired checks if an action timestamp is older than ActionTTL
func IsExpired(actionCreatedAt time.Time) bool {
	age := time.Since(actionCreatedAt)
	return age > ActionTTL
}

// ValidateTimestamp validates an action timestamp and returns an error if invalid
func ValidateTimestamp(actionCreatedAt time.Time) error {
	now := time.Now()

	// Check if timestamp is zero/missing
	if actionCreatedAt.IsZero() {
		return fmt.Errorf("action timestamp is missing or zero")
	}

	// Check if timestamp is in the future (with 10 second buffer for clock skew)
	if actionCreatedAt.After(now.Add(10 * time.Second)) {
		return fmt.Errorf("action timestamp is in the future: %v (now: %v)", actionCreatedAt, now)
	}

	// Check if timestamp is expired
	if IsExpired(actionCreatedAt) {
		return fmt.Errorf("action timestamp is expired: %v (age: %v, TTL: %v)",
			actionCreatedAt, time.Since(actionCreatedAt), ActionTTL)
	}

	return nil
}

// WasExecuted checks if an action was already executed
func (s *ActionStore) WasExecuted(key ActionKey) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.executed[key.String()]
	return exists
}

// MarkExecuted marks an action as executed with the given status and message
func (s *ActionStore) MarkExecuted(key ActionKey, status, message string, executedAt int64, receivedAt int64, actionCreatedAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.executed[key.String()] = ActionRecord{
		Key:             key,
		Status:          status,
		Message:         message,
		ExecutedAt:      executedAt,
		ReceivedAt:      receivedAt,
		ActionCreatedAt: actionCreatedAt,
	}
}

// GetRecord retrieves the execution record for an action
func (s *ActionStore) GetRecord(key ActionKey) (ActionRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.executed[key.String()]
	return record, exists
}

// GetAll returns all execution records
func (s *ActionStore) GetAll() []ActionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := make([]ActionRecord, 0, len(s.executed))
	for _, record := range s.executed {
		records = append(records, record)
	}
	return records
}

// Count returns the number of executed actions
func (s *ActionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.executed)
}

// cleanupLoop periodically removes expired action records from memory
func (s *ActionStore) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Infof("Action store cleanup loop stopped")
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup removes action records that are older than RecordRetentionTTL
// This keeps executed actions in memory for inspection and debugging
func (s *ActionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-RecordRetentionTTL).Unix()
	removed := 0

	for key, record := range s.executed {
		// Remove if the action was created more than RecordRetentionTTL ago
		if record.ActionCreatedAt > 0 && record.ActionCreatedAt < cutoff {
			delete(s.executed, key)
			removed++
		}
	}

	if removed > 0 {
		log.Debugf("Cleaned up %d expired action records from memory (remaining: %d)", removed, len(s.executed))
	}
}

// Stop stops the cleanup goroutine
func (s *ActionStore) Stop() {
	close(s.stopCh)
}
