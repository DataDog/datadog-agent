// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ActionTTL is how long action timestamps are considered valid.
	ActionTTL = 1 * time.Minute
	// RecordRetentionTTL is how long action records are kept in memory.
	RecordRetentionTTL = 24 * time.Hour
	// CleanupInterval is how often expired records are purged.
	CleanupInterval = 30 * time.Second
)

// ActionRecord stores information about a processed action.
type ActionRecord struct {
	Key             ActionKey
	Status          string
	Message         string
	ExecutedAt      int64
	ReceivedAt      int64
	ActionCreatedAt int64
	ClaimedAt       int64
}

// ActionStore tracks processed actions in-memory to prevent duplicate execution.
type ActionStore struct {
	executed map[string]ActionRecord
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewActionStore creates a new ActionStore and starts the background cleanup goroutine.
func NewActionStore(ctx context.Context) *ActionStore {
	s := &ActionStore{
		executed: make(map[string]ActionRecord),
		stopCh:   make(chan struct{}),
	}
	go s.cleanupLoop(ctx)
	log.Debugf("[HelmActions] Action store initialized (TTL=%v, retention=%v, cleanup=%v)",
		ActionTTL, RecordRetentionTTL, CleanupInterval)
	return s
}

// ValidateTimestamp returns an error if ts is zero, in the future, or older than ActionTTL.
func ValidateTimestamp(ts time.Time) error {
	if ts.IsZero() {
		return errors.New("action timestamp is missing or zero")
	}
	now := time.Now()
	if ts.After(now.Add(10 * time.Second)) {
		return fmt.Errorf("action timestamp is in the future: %v (now: %v)", ts, now)
	}
	if time.Since(ts) > ActionTTL {
		return fmt.Errorf("action timestamp is expired: %v (age: %v, TTL: %v)", ts, time.Since(ts), ActionTTL)
	}
	return nil
}

// Claim tries to claim an action for execution. Returns false if already claimed.
func (s *ActionStore) Claim(key ActionKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.executed[key.String()]; exists {
		return false
	}
	s.executed[key.String()] = ActionRecord{
		Key:       key,
		Status:    StatusClaimed,
		Message:   "action claimed",
		ClaimedAt: time.Now().Unix(),
	}
	return true
}

// MarkExecuted updates the record for a previously claimed action.
func (s *ActionStore) MarkExecuted(key ActionKey, status, message string, executedAt, receivedAt, actionCreatedAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	claimedAt := s.executed[key.String()].ClaimedAt
	s.executed[key.String()] = ActionRecord{
		Key:             key,
		Status:          status,
		Message:         message,
		ExecutedAt:      executedAt,
		ReceivedAt:      receivedAt,
		ActionCreatedAt: actionCreatedAt,
		ClaimedAt:       claimedAt,
	}
}

// GetRecord retrieves the execution record for an action.
func (s *ActionStore) GetRecord(key ActionKey) (ActionRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, exists := s.executed[key.String()]
	return record, exists
}

// GetAll returns all execution records.
func (s *ActionStore) GetAll() []ActionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Collect(maps.Values(s.executed))
}

// Count returns the number of tracked action records.
func (s *ActionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.executed)
}

func (s *ActionStore) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Debugf("[HelmActions] Action store cleanup loop stopped")
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *ActionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-RecordRetentionTTL).Unix()
	removed := 0
	for k, r := range s.executed {
		ts := r.ActionCreatedAt
		if ts == 0 {
			ts = r.ExecutedAt
		}
		if (ts > 0 && ts < cutoff) || (r.ClaimedAt > 0 && r.ClaimedAt < cutoff) {
			delete(s.executed, k)
			removed++
		}
	}
	if removed > 0 {
		log.Debugf("[HelmActions] Cleaned up %d expired action records (remaining: %d)", removed, len(s.executed))
	}
}

// Stop shuts down the cleanup goroutine.
func (s *ActionStore) Stop() {
	close(s.stopCh)
}
