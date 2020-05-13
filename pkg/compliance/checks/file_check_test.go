// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
)

func TestFileCheck(t *testing.T) {
	reporter := &compliance.MockReporter{}

	const (
		framework    = "cis-docker"
		version      = "1.2.0"
		ruleID       = "rule"
		resourceID   = "host"
		resourceType = "docker"
	)

	fc := &fileCheck{
		baseCheck: baseCheck{
			id:        check.ID("check-1"),
			interval:  time.Minute,
			framework: framework,
			version:   version,

			ruleID:       ruleID,
			resourceType: resourceType,
			resourceID:   resourceID,
			reporter:     reporter,
		},
		File: &compliance.File{
			Path: "./testdata/644.dat",
			Report: compliance.Report{
				{
					Attribute: "permissions",
				},
			},
		},
	}
	reporter.On(
		"Report",
		&compliance.RuleEvent{
			RuleID:       ruleID,
			Framework:    framework,
			Version:      version,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Data: compliance.KV{
				"permissions": "644",
			},
		},
	).Once()

	err := fc.Run()
	assert.NoError(t, err)
}
