// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
)

// checkable abstracts a resource check
type checkable interface {
	check(env env.Env) []*compliance.Report
}

// checkableList abstracts a list of resource checks
type checkableList []checkable

// check implements checkable interface for checkableList
// note that this implements AND for all checkables in a check:
// failure or error from a single checkable fails the check, all checkables must
// return Passed in order for the check to be successful.
func (list checkableList) check(env env.Env) []*compliance.Report {
	var (
		reports []*compliance.Report
		last    *compliance.Report
		succeed = true
	)

	for _, c := range list {
		for _, last = range c.check(env) {
			if !last.Passed {
				succeed = false

				if len(reports) < env.MaxEventsPerRun() {
					reports = append(reports, last)
				} else {
					break
				}
			}
		}
	}

	if succeed {
		return []*compliance.Report{last}
	}

	return reports
}
