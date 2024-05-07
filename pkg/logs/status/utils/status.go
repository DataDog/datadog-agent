// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status == isPending
}

// IsSuccess returns whether the current status is a success.
func (s *LogStatus) IsSuccess() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status == isSuccess
}

// IsError returns whether the current status is an error.
func (s *LogStatus) IsError() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status == isError
}

// GetError returns the error.
func (s *LogStatus) GetError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Dump provides a single-line dump of the status, for debugging purposes.
func (s *LogStatus) Dump() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var status string
	switch s.status {
	case isPending:
		status = "isPending"
	case isSuccess:
		status = "isSuccess"
	case isError:
		status = "isError"
	default:
		status = fmt.Sprintf("%d", s.status)
	}
	return fmt.Sprintf("&LogStatus{status: %s, err: %#v}", status, s.err)
}

// String returns a human readable representation of the status.
func (s *LogStatus) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.status {
	case isPending:
		return "pending"
	case isSuccess:
		return "success"
	case isError:
		return "error"
	default:
		return fmt.Sprintf("unknown status: %d", s.status)
	}
}
