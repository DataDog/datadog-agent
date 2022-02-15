// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
)

// checkable abstracts a resource check
type checkable interface {
	check(env env.Env) []*compliance.Report
}

// checkableList abstracts a list of resource checks
type checkableList []checkable

var (
	// ErrTruncatedResults is reported when the reports list is truncated
	ErrTruncatedResults = errors.New("truncated result")
)

// check implements checkable interface for checkableList
// note that this implements AND for all checkables in a check:
// failure or error from a single checkable fails the check, all checkables must
// return Passed in order for the check to be successful.
func (list checkableList) check(env env.Env) []*compliance.Report {
	var (
		reports []*compliance.Report
	)

LOOP:
	for _, c := range list {
		for i, report := range c.check(env) {
			if len(reports) >= env.MaxEventsPerRun() {
				// generate an error report to notify that the results were
				// truncated
				reports = append(reports, &compliance.Report{
					Passed: false,
					Error:  ErrTruncatedResults,
					Data: map[string]interface{}{
						"truncated": len(reports) - (i + 1),
					},
				})
				break LOOP
			}
			reports = append(reports, report)
		}
	}

	return reports
}
