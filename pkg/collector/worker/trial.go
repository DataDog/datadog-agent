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
	trialResultCallbacks []func(id checkid.ID, ok bool)
)

// SetTrialResultCallback registers a function to be called after each
// trial-mode check run with the run outcome. Multiple callbacks may be
// registered; they are called in registration order.
// This is intended to be called once during agent startup by AutoConfig.
func SetTrialResultCallback(fn func(id checkid.ID, ok bool)) {
	trialMu.Lock()
	defer trialMu.Unlock()
	trialResultCallbacks = append(trialResultCallbacks, fn)
}

// notifyTrialResult invokes all registered trial-result callbacks.
func notifyTrialResult(id checkid.ID, ok bool) {
	trialMu.RLock()
	defer trialMu.RUnlock()
	for _, fn := range trialResultCallbacks {
		fn(id, ok)
	}
}
