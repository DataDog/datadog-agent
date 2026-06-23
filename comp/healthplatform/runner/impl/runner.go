// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package runnerimpl implements the health platform runner component.
package runnerimpl

import (
	"fmt"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	registrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

type runner struct {
	log      log.Component
	registry registrydef.Component
	store    storedef.Component
}

// Requires defines the dependencies for the runner.
type Requires struct {
	Log      log.Component
	Registry registrydef.Component
	Store    storedef.Component
}

// NewComponent creates a new runner instance.
func NewComponent(reqs Requires) runnerdef.Component {
	return &runner{
		log:      reqs.Log,
		registry: reqs.Registry,
		store:    reqs.Store,
	}
}

// Run executes fn once with panic recovery, translates each emitted IssueReport
// into a proto Issue via the registry, forwards it to the store, and returns
// the slice of IssueIds that were successfully reported.
//
// If fn returns both reports and a non-nil error, the reports emitted before the
// error are still forwarded — partial results are not silently dropped. Callers
// should treat a non-nil error as a signal that the check may be incomplete and
// must not use the returned IDs for issue-state diffs.
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
		if report.Source == "" {
			report.Source = source
		}
		issue := r.toProto(report)
		if reportErr := r.store.ReportIssue(issue); reportErr != nil {
			r.log.Warnf("failed to report issue %s from %s: %v", report.IssueID, source, reportErr)
		} else {
			issueIDs = append(issueIDs, report.IssueID)
		}
	}

	return issueIDs, retErr
}

// toProto builds a proto Issue from an IssueReport. If the registry has a
// template for report.IssueName, that template is used to populate the proto
// fields; otherwise a minimal proto is built from the report fields directly.
func (r *runner) toProto(report runnerdef.IssueReport) *healthplatformpayload.Issue {
	if tmpl, ok := r.registry.GetTemplate(report.IssueName); ok {
		issue, err := tmpl.BuildIssue(report.Context)
		if err == nil && issue != nil {
			issue.Id = report.IssueID
			if len(report.Tags) > 0 {
				issue.Tags = append(issue.Tags, report.Tags...)
			}
			return issue
		}
		r.log.Warnf("runner: failed to build issue %s from registry: %v; using minimal proto", report.IssueName, err)
	}
	return &healthplatformpayload.Issue{
		Id:        report.IssueID,
		IssueName: report.IssueName,
		Title:     report.IssueName,
		Source:    report.Source,
		Tags:      report.Tags,
	}
}
