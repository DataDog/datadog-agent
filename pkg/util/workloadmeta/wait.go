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
	// DefaultTimeout is a sensible default for WaitForInitialization: the store gives its
	// collectors 30s to start before reporting itself as initialized regardless, so waiting much
	// longer than that brings nothing.
	DefaultTimeout = 40 * time.Second

	// pollInterval is how often the readiness of the workloadmeta store is polled.
	pollInterval = 100 * time.Millisecond
)

// WaitForInitialization blocks until the workloadmeta store reports itself as initialized, and
// reports whether it did. A timeout of 0 or less does not wait at all and returns false.
//
// One-shot commands that build their own workloadmeta store start its collectors asynchronously.
// A command reading from the store before its collectors have published their entities gets no
// data rather than an error, so waiting here is what makes such commands report anything useful.
//
// timeout is when to give up, not a hard bound on how long this blocks: probing the store takes a
// lock that collector startup holds, so a probe can return after the deadline, in which case the
// state it found is reported rather than discarded.
//
// Progress is reported through the log component, never on stdout, which commands such as
// `agent check --json` expect to be valid JSON. Callers needing to surface a failed wait
// regardless of the configured log level should do so from the returned value.
func WaitForInitialization(wmeta workloadmeta.Component, timeout time.Duration, logger log.Component) bool {
	if timeout <= 0 {
		return false
	}

	// Started before the first probe, so that the deadline covers it too.
	start := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	if wmeta.IsInitialized() {
		return true
	}

	logger.Info("Waiting for workloadmeta to be initialized...")

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
