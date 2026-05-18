// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import "sync"

// NewTrialModeCheck wraps a check scheduled by configuration discovery.
//
// Trial mode is runner metadata: checks do not need to know about it. The
// worker uses the optional IsTrialMode/ClearTrialMode methods on this wrapper
// to suppress discovery-probe failures until the first successful run.
func NewTrialModeCheck(inner Check) Check {
	return &trialModeCheck{
		Check:     inner,
		trialMode: true,
	}
}

type trialModeCheck struct {
	Check

	mu        sync.Mutex
	trialMode bool
}

// IsTrialMode returns true until the worker promotes the check after a
// successful discovery-probe run.
func (c *trialModeCheck) IsTrialMode() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.trialMode
}

// ClearTrialMode promotes the wrapped check out of discovery-probe mode.
func (c *trialModeCheck) ClearTrialMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trialMode = false
}
