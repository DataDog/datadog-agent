// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"sync"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

var (
	trialMu              sync.RWMutex
	trialResultCallbacks []func(id checkid.ID, ok bool) bool
)

type trialModeCheck interface {
	IsTrialMode() bool
	ClearTrialMode()
}

// RegisterTrialResultCallback registers a function to be called after each
// trial-mode check run with the run outcome. The callback returns true when
// the worker should suppress the result from normal integration reporting.
// Multiple callbacks may be registered; they are called in registration order,
// and any suppressing callback suppresses the result.
// This is intended to be called once during agent startup by AutoConfig.
func RegisterTrialResultCallback(fn func(id checkid.ID, ok bool) bool) {
	trialMu.Lock()
	defer trialMu.Unlock()
	trialResultCallbacks = append(trialResultCallbacks, fn)
}

// notifyTrialResult invokes all registered trial-result callbacks and returns
// whether the worker should suppress the result from normal integration
// reporting. Failed trial results are suppressed by default.
func notifyTrialResult(id checkid.ID, ok bool) bool {
	trialMu.RLock()
	callbacks := make([]func(id checkid.ID, ok bool) bool, len(trialResultCallbacks))
	copy(callbacks, trialResultCallbacks)
	trialMu.RUnlock()

	suppress := !ok
	for _, fn := range callbacks {
		if fn(id, ok) {
			suppress = true
		}
	}
	return suppress
}
