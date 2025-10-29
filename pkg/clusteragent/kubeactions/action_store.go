// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"fmt"
	"sync"
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

// ActionRecord stores information about an executed action
type ActionRecord struct {
	Key       ActionKey
	Status    string // "success", "failed", "skipped"
	Message   string
	Timestamp int64 // Unix timestamp when action was executed
}

// ActionStore tracks executed actions to prevent duplicate execution
type ActionStore struct {
	executed map[string]ActionRecord
	mu       sync.RWMutex
}

// NewActionStore creates a new ActionStore
func NewActionStore() *ActionStore {
	return &ActionStore{
		executed: make(map[string]ActionRecord),
	}
}

// WasExecuted checks if an action was already executed
func (s *ActionStore) WasExecuted(key ActionKey) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.executed[key.String()]
	return exists
}

// MarkExecuted marks an action as executed with the given status and message
func (s *ActionStore) MarkExecuted(key ActionKey, status, message string, timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.executed[key.String()] = ActionRecord{
		Key:       key,
		Status:    status,
		Message:   message,
		Timestamp: timestamp,
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
