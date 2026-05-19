// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package runnerimpl implements the health platform runner component.
package runnerimpl

import (
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

type runner struct {
	log   log.Component
	store storedef.Component
}

// Requires defines the dependencies for the runner.
type Requires struct {
	Log   log.Component
	Store storedef.Component
}

// New creates a new runner instance.
func New(reqs Requires) runnerdef.Component {
	return &runner{
		log:   reqs.Log,
		store: reqs.Store,
	}
}

// Run executes fn once with panic recovery, forwards each emitted IssueReport to
// the store, fills Source if empty, and returns the slice of IssueIds that were
// successfully reported.
//
// If fn returns both reports and a non-nil error, the reports emitted before the
// error are still forwarded to the store — this is intentional so that partial
// results are not silently dropped. Callers should treat a non-nil error as a
// signal that the check may be incomplete and must not use the returned IDs for
// issue-state diffs.
func (r *runner) Run(source string, fn runnerdef.HealthCheckFunc) (issueIDs []string, retErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("health check panic: %v", rec)
			r.log.Errorf("health check %s panicked: %v", source, rec)
			// Zero issueIDs so the scheduler does not treat partial results
			// from a mid-loop panic as successfully reported.
			issueIDs = nil
		}
	}()

	reports, err := fn()
	if err != nil {
		r.log.Warnf("health check %s returned error: %v", source, err)
		retErr = err
	}

	for _, report := range reports {
		// report is a value copy from the range — safe to modify Source directly.
		if report.Source == "" {
			report.Source = source
		}
		if reportErr := r.store.ReportIssue(report); reportErr != nil {
			r.log.Warnf("failed to report issue %s from %s: %v", report.IssueID, source, reportErr)
		} else {
			issueIDs = append(issueIDs, report.IssueID)
		}
	}

	return issueIDs, retErr
}
