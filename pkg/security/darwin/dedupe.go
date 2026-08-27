// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// dedupeMaxPids bounds memory: tokens are pids, and macOS wraps at 99999.
	dedupeMaxPids = 4096
	// dedupePeriod is the window within which a repeated (rule, pid) is treated as
	// the same finding. It only has to be long enough to cover a shim re-exec,
	// which happens within microseconds, so it is kept short: genuinely distinct
	// activity at the same pid a second later is still reported.
	dedupePeriod = 2 * time.Second
)

// signalDeduper collapses repeated matches of the same rule against the same
// process into one signal.
//
// This exists because of a macOS detail rather than a general desire to
// rate-limit. /bin/sh is bash in sh-compat mode and immediately re-execs itself as
// /bin/bash, so a single shell invocation produces two exec events at the same
// pid. Both are real execs and both genuinely match a rule on shell names, so the
// engine fires twice and one action appears as two signals. Deduping on pid is the
// narrowest fix that does not require the rule to know about Apple's shell
// packaging.
//
// It uses events.TokenLimiter, the mechanism CWS already has for this, keyed on
// process.pid, with one limiter per rule so that different rules about the same
// process are still separate findings.
type signalDeduper struct {
	mu       sync.Mutex
	limiters map[string]*events.TokenLimiter
	// Suppressed counts collapsed duplicates, so the effect is measurable rather
	// than invisible.
	Suppressed uint64
}

// newSignalDeduper returns a deduper. Limiters are created per rule on first use,
// so no rule list is needed up front.
func newSignalDeduper() (*signalDeduper, error) {
	// Validate the field once here rather than discovering a typo per rule.
	if _, err := events.NewTokenLimiter(1, 1, dedupePeriod, dedupeFields()); err != nil {
		return nil, fmt.Errorf("dedupe limiter: %w", err)
	}
	return &signalDeduper{limiters: make(map[string]*events.TokenLimiter)}, nil
}

// dedupeFields is the token: one signal per process per rule per period.
func dedupeFields() []eval.Field {
	return []eval.Field{"process.pid"}
}

// allow reports whether this match should be sent.
func (d *signalDeduper) allow(ruleID string, event *model.Event) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	limiter, ok := d.limiters[ruleID]
	if !ok {
		var err error
		limiter, err = events.NewTokenLimiter(dedupeMaxPids, 1, dedupePeriod, dedupeFields())
		if err != nil {
			// Constructing it already succeeded once in newSignalDeduper, so this
			// should not happen; failing open is the right bias for a detection.
			return true
		}
		d.limiters[ruleID] = limiter
	}

	if limiter.Allow(event) {
		return true
	}

	d.Suppressed++
	return false
}
