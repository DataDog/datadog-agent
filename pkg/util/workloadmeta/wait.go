// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadmeta provides helpers to work with the workloadmeta store.
package workloadmeta

import (
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

const (
	// DefaultTimeout is a sensible upper bound for WaitForInitialization. The store waits up to
	// 30s for its collectors to start before doing its first pull and reporting itself as
	// initialized regardless, so waiting much longer than that brings nothing.
	DefaultTimeout = 40 * time.Second

	// pollInterval is how often the readiness of the workloadmeta store is polled.
	pollInterval = 100 * time.Millisecond
)

// WaitForInitialization blocks until the workloadmeta store reports itself as initialized, and
// reports whether it did before timeout expired. A timeout of 0 or less returns immediately.
//
// One-shot commands that build their own workloadmeta store start its collectors
// asynchronously, and a command that reads from the store before that happened gets no data
// rather than an error.
//
// This waits on workloadmeta's own best-effort readiness signal, the same one autodiscovery
// waits on: the store reports itself as initialized once at least one of its collectors has
// started — or, at worst, once it stops waiting for them before its first pull — and that first
// pull round has been kicked off. It is not a guarantee that every collector has started, so it
// closes the multi-second startup race rather than making it impossible.
//
// Nothing is written directly to stdout, which commands such as `agent check --json` expect to
// be valid JSON: progress goes through the log component, and is therefore subject to the log
// level the command was configured with. Callers that need to surface a failed wait to the user
// regardless of that level should do so from the returned value.
func WaitForInitialization(wmeta workloadmeta.Component, timeout time.Duration, logger log.Component) bool {
	if wmeta.IsInitialized() {
		return true
	}
	if timeout <= 0 {
		return false
	}

	logger.Info("Waiting for workloadmeta to be initialized...")
	start := time.Now()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-deadline.C:
			logger.Warnf("Workloadmeta is not ready after %v, proceeding anyway: results may be incomplete or empty", timeout)
			return false
		case <-ticker.C:
			if wmeta.IsInitialized() {
				logger.Infof("Workloadmeta is ready after %v, proceeding", time.Since(start))
				return true
			}
		}
	}
}
