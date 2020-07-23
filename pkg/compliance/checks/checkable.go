// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
)

// report is a report produced by a checkable
type report struct {
	data   event.Data
	passed bool
}

// checkable abstracts a resource check
type checkable interface {
	check(env env.Env) (*report, error)
}

type checkableList []checkable

// check implements checkable interface for checkableList
func (list checkableList) check(env env.Env) (*report, error) {
	var (
		result *report
		err    error
	)

	for _, c := range list {
		result, err = c.check(env)
		if err != nil || !result.passed {
			continue
		}
	}
	return result, err
}
