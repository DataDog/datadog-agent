// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"fmt"
	"sync"
)

type status int

const (
	isPending status = iota
	isSuccess
	isError
)

// LogStatus tracks errors and success.
type LogStatus struct {
	status status
	err    string
	mu     *sync.Mutex
}

// NewLogStatus creates a new log status.
func NewLogStatus() *LogStatus {
	return &LogStatus{
		status: isPending,
		mu:     &sync.Mutex{},
	}
}

// Success sets the status to success.
func (s *LogStatus) Success() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = isSuccess
	s.err = ""
}

// Error records the given error and invalidates the source.
func (s *LogStatus) Error(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = isError
	s.err = fmt.Sprintf("Error: %s", err.Error())
}

// IsPending returns whether the current status is not yet determined.
func (s *LogStatus) IsPending() bool {
	return s.status == isPending
}

// IsSuccess returns whether the current status is a success.
func (s *LogStatus) IsSuccess() bool {
	return s.status == isSuccess
}

// IsError returns whether the current status is an error.
func (s *LogStatus) IsError() bool {
	return s.status == isError
}

// GetError returns the error.
func (s *LogStatus) GetError() string {
	return s.err
}
