// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package connectivity tracks whether this process can reach the Datadog
// Cluster Agent over the network. Cluster Agent clients record transport-level
// outcomes; the health platform reads them to report sustained outages. It is
// a leaf package because the health platform already depends on the Cluster
// Agent client (through cluster name resolution), so the client can't import it.
package connectivity

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const maxErrorLength = 512

// Failure describes an ongoing Cluster Agent connectivity failure.
type Failure struct {
	Since     time.Time
	LastError string
}

// Tracker records Cluster Agent connectivity outcomes.
type Tracker struct {
	mu      sync.Mutex
	now     func() time.Time
	failure *Failure
}

// NewTracker returns a Tracker that reads the time from now.
func NewTracker(now func() time.Time) *Tracker {
	return &Tracker{now: now}
}

// Default is the process-wide tracker fed by the Cluster Agent clients.
var Default = NewTracker(time.Now)

// ReportFailure records a network-level failure to reach the Cluster Agent on Default.
func ReportFailure(err error) {
	Default.ReportFailure(err)
}

// ReportSuccess records that the Cluster Agent answered a request on Default.
func ReportSuccess() {
	Default.ReportSuccess()
}

// ReportFailure records a network-level failure to reach the Cluster Agent.
func (t *Tracker) ReportFailure(err error) {
	msg := "unknown error"
	if err != nil {
		msg = scrubber.ScrubLine(err.Error())
	}
	if len(msg) > maxErrorLength {
		msg = msg[:maxErrorLength] + "..."
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failure == nil {
		t.failure = &Failure{Since: t.now()}
	}
	t.failure.LastError = msg
}

// ReportSuccess records that the Cluster Agent answered a request, whatever
// the response status: an answer proves it is reachable.
func (t *Tracker) ReportSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failure = nil
}

// FailingFor returns the current failure if the Cluster Agent has been
// unreachable for at least threshold.
func (t *Tracker) FailingFor(threshold time.Duration) (Failure, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failure == nil || t.now().Sub(t.failure.Since) < threshold {
		return Failure{}, false
	}
	return *t.failure, true
}
