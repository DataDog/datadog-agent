// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"sync"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// trialRegistry tracks consecutive failures of trial-mode (discovery)
// checks. After threshold consecutive failures, recordResult returns true
// to signal the check should be unscheduled.
type trialRegistry struct {
	threshold int
	mu        sync.Mutex
	counts    map[checkid.ID]int
}

func newTrialRegistry(threshold int) *trialRegistry {
	return &trialRegistry{
		threshold: threshold,
		counts:    map[checkid.ID]int{},
	}
}

// recordResult records a check run outcome. Returns true if the failure
// count has reached threshold and the check should be unscheduled.
func (r *trialRegistry) recordResult(id checkid.ID, ok bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ok {
		delete(r.counts, id)
		return false
	}
	r.counts[id]++
	return r.counts[id] >= r.threshold
}

// forget drops any tracked state for id. Called on unschedule.
func (r *trialRegistry) forget(id checkid.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.counts, id)
}
