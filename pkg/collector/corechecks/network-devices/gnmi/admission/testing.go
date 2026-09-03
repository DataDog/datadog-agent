// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package admission

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
)

// ResetGateForTesting destroys the singleton gate. Only use in tests.
func ResetGateForTesting() {
	gateOnce = sync.Once{}
	gateInstance = nil
}

// SetPaceForTesting overrides pacing parameters on the singleton gate.
func SetPaceForTesting(interval time.Duration, burst int) {
	g := Gate()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paceInterval = interval
	g.paceBurst = burst
	g.tokens = burst
	g.lastRefill = time.Time{}
}

// SetFDRetryIntervalForTesting overrides the FD guard retry interval.
func SetFDRetryIntervalForTesting(interval time.Duration) {
	g := Gate()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fdRetryInterval = interval
}

// SetFileStatsFuncForTesting overrides process file stats retrieval.
func SetFileStatsFuncForTesting(fn func() (*procfilestats.ProcessFileStats, error)) {
	g := Gate()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.getFileStats = fn
}

// AddPacingTokensForTesting adds pacing tokens for tests waiting on startup pacing.
func AddPacingTokensForTesting(tokens int) {
	g := Gate()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tokens += tokens
	if g.tokens > g.paceBurst {
		g.tokens = g.paceBurst
	}
	g.cond.Broadcast()
}
