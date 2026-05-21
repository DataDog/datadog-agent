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
	trialMu             sync.RWMutex
	trialResultCallback func(id checkid.ID, ok bool) bool
)

type trialModeCheck interface {
	IsTrialMode() bool
	ClearTrialMode()
}

// RegisterTrialResultCallback registers a function to be called after each
// trial-mode check run with the run outcome. The callback returns true when
// the worker should suppress the result from normal integration reporting.
// This is intended to be called once during agent startup by AutoConfig.
func RegisterTrialResultCallback(fn func(id checkid.ID, ok bool) bool) {
	trialMu.Lock()
	defer trialMu.Unlock()
	trialResultCallback = fn
}

// notifyTrialResult invokes the registered trial-result callback and returns
// whether the worker should suppress the result from normal integration
// reporting. Failed trial results are suppressed by default, even when no
// callback is registered.
func notifyTrialResult(id checkid.ID, ok bool) bool {
	trialMu.RLock()
	fn := trialResultCallback
	trialMu.RUnlock()

	suppress := !ok
	if fn != nil && fn(id, ok) {
		suppress = true
	}
	return suppress
}
