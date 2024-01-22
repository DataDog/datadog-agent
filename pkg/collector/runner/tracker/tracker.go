// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package tracker

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// withCheckFunc is a closure you can run on a mutex-locked check
type withCheckFunc func(check.Check)

// withRunningChecksFunc is a closure you can run on a mutex-locked list of
// running checks
type withRunningChecksFunc func(map[checkid.ID]check.Check)

// RunningChecksTracker is an object that keeps a thread-safe track of
// all the running checks
type RunningChecksTracker struct {
	runningChecks map[checkid.ID]check.Check // The list of checks running
	accessLock    sync.RWMutex               // To control races on runningChecks
}

// NewRunningChecksTracker is a contructor for a RunningChecksTracker
func NewRunningChecksTracker() *RunningChecksTracker {
	return &RunningChecksTracker{
		runningChecks: make(map[checkid.ID]check.Check),
	}
}

// Check returns a check in the running check list, if it can be found
func (t *RunningChecksTracker) Check(id checkid.ID) (check.Check, bool) {
	t.accessLock.RLock()
	defer t.accessLock.RUnlock()

	check, found := t.runningChecks[id]
	return check, found
}

// AddCheck adds a check to the list of running checks if the check
// isn't already added. Method returns a boolean if the addition was
// successful.
func (t *RunningChecksTracker) AddCheck(check check.Check) bool {
	t.accessLock.Lock()
	defer t.accessLock.Unlock()

	if _, found := t.runningChecks[check.ID()]; found {
		return false
	}

	t.runningChecks[check.ID()] = check
	return true
}

// DeleteCheck removes a check from the list of running checks
func (t *RunningChecksTracker) DeleteCheck(id checkid.ID) {
	t.accessLock.Lock()
	defer t.accessLock.Unlock()

	delete(t.runningChecks, id)
}

// WithRunningChecks takes in a function to execute in the context of a locked
// state of the checks tracker
func (t *RunningChecksTracker) WithRunningChecks(closureFunc withRunningChecksFunc) {
	closureFunc(t.RunningChecks())
}

// RunningChecks returns a list of all the running checks
func (t *RunningChecksTracker) RunningChecks() map[checkid.ID]check.Check {
	t.accessLock.RLock()
	defer t.accessLock.RUnlock()

	clone := make(map[checkid.ID]check.Check)
	for key, val := range t.runningChecks {
		clone[key] = val
	}

	return clone
}

// WithCheck takes in a function to execute in the context of a locked
// state of the checks tracker on a single check
func (t *RunningChecksTracker) WithCheck(id checkid.ID, closureFunc withCheckFunc) bool {
	t.accessLock.RLock()
	defer t.accessLock.RUnlock()

	check, found := t.runningChecks[id]

	if !found {
		return false
	}

	closureFunc(check)

	return true
}
