// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
)

func TestFileCheck(t *testing.T) {
	reporter := &compliance.MockReporter{}

	fc := &fileCheck{
		baseCheck: newTestBaseCheck(reporter),
		File: &compliance.File{
			Path: "./testdata/644.dat",
			Report: compliance.Report{
				{
					Property: "permissions",
					Kind:     "attribute",
				},
			},
		},
	}
	reporter.On(
		"Report",
		newTestRuleEvent(
			nil,
			compliance.KV{
				"permissions": "644",
			},
		),
	).Once()

	err := fc.Run()
	assert.NoError(t, err)
}
