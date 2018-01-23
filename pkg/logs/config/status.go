// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"sync"
)

// LogStatus tracks errors and success.
type LogStatus struct {
	pending bool
	errors  []string
	mu      *sync.Mutex
}

// NewLogStatus creates a new log status.
func NewLogStatus() *LogStatus {
	return &LogStatus{
		pending: true,
		mu:      &sync.Mutex{},
	}
}

// Success sets the status to success.
func (s *LogStatus) Success() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = false
	s.errors = make([]string, 0)
}

// Error records the given error.
func (s *LogStatus) Error(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = false
	s.errors = append(s.errors, fmt.Sprintf("Error: %s", err.Error()))
}

// IsPending returns whether the current status is not yet determined.
func (s *LogStatus) IsPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending
}

// IsSuccess returns whether the current status is a success.
func (s *LogStatus) IsSuccess() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.pending && len(s.errors) == 0
}

// HasErrors returns whether the current status has errors.
func (s *LogStatus) HasErrors() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.errors) > 0
}

// GetErrors returns the errors.
func (s *LogStatus) GetErrors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errors
}
