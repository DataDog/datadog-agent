// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import (
	"fmt"
	"sync"
)

// Tracker Tracks errors and success operations for a given source
// it's designed to be thread safe
type Tracker struct {
	sourceType string
	status     string
	mu         *sync.Mutex
}

// NewTracker returns a new Tracker
func NewTracker(sourceType string) *Tracker {
	return &Tracker{
		sourceType: sourceType,
		status:     "Pending",
		mu:         &sync.Mutex{},
	}
}

// TrackError formats the error and keeps it in status
func (t *Tracker) TrackError(err error) {
	t.mu.Lock()
	t.status = fmt.Sprintf("Error: %s", err.Error())
	t.mu.Unlock()
}

// TrackSuccess formats the success operation and keeps it in status
func (t *Tracker) TrackSuccess() {
	t.mu.Lock()
	t.status = "OK"
	t.mu.Unlock()
}

// GetSource returns the source computed from sourceType and status
func (t *Tracker) GetSource() Source {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Source{
		Type:   t.sourceType,
		Status: t.status,
	}
}
