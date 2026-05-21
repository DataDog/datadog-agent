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
	trialResultCallbacks []func(id checkid.ID, ok bool) TrialResultDecision
)

type trialModeCheck interface {
	IsTrialMode() bool
	ClearTrialMode()
}

// TrialResultDecision tells the worker how to handle a trial-mode run result.
type TrialResultDecision int

const (
	// TrialResultContinue keeps the check in trial mode and suppresses the
	// run outcome from normal integration reporting.
	TrialResultContinue TrialResultDecision = iota
	// TrialResultPromote clears trial mode after a successful probe so future
	// runs are reported like regular check runs.
	TrialResultPromote
	// TrialResultRetire keeps the result suppressed because AutoConfig is
	// removing, or has already removed, the trial check.
	TrialResultRetire
)

// RegisterTrialResultCallback registers a function to be called after each
// trial-mode check run with the run outcome. The callback returns the worker
// disposition for that result. Multiple callbacks may be registered; they are
// called in registration order, and the most suppressive decision wins.
// This is intended to be called once during agent startup by AutoConfig.
func RegisterTrialResultCallback(fn func(id checkid.ID, ok bool) TrialResultDecision) {
	trialMu.Lock()
	defer trialMu.Unlock()
	trialResultCallbacks = append(trialResultCallbacks, fn)
}

// notifyTrialResult invokes all registered trial-result callbacks and returns
// the merged worker disposition.
func notifyTrialResult(id checkid.ID, ok bool) TrialResultDecision {
	trialMu.RLock()
	callbacks := make([]func(id checkid.ID, ok bool) TrialResultDecision, len(trialResultCallbacks))
	copy(callbacks, trialResultCallbacks)
	trialMu.RUnlock()

	decision := defaultTrialResultDecision(ok)
	for _, fn := range callbacks {
		decision = mergeTrialResultDecision(decision, fn(id, ok))
	}
	return decision
}

func defaultTrialResultDecision(ok bool) TrialResultDecision {
	if ok {
		return TrialResultPromote
	}
	return TrialResultContinue
}

func mergeTrialResultDecision(a, b TrialResultDecision) TrialResultDecision {
	if a == TrialResultRetire || b == TrialResultRetire {
		return TrialResultRetire
	}
	if a == TrialResultContinue || b == TrialResultContinue {
		return TrialResultContinue
	}
	return TrialResultPromote
}
