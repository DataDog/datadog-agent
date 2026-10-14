// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inframode

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type checkEventsCloudCostOnlyLinuxSuite struct {
	checkEventsSuite
}

type checkEventsFullLinuxSuite struct {
	checkEventsSuite
}

func TestCheckEventsInfraModeCloudCostOnlyLinux(t *testing.T) {
	t.Parallel()

	suite := &checkEventsCloudCostOnlyLinuxSuite{
		checkEventsSuite{
			infraMode:  "cloud_cost_only",
			expectMark: true,
			markTag:    infrastructureModeTag,
		},
	}
	e2e.Run(t, suite, suite.getSuiteOptions("check-events-ccm")...)
}

func TestCheckEventsInfraModeFullLinux(t *testing.T) {
	t.Parallel()

	suite := &checkEventsFullLinuxSuite{
		checkEventsSuite{
			infraMode:  "full",
			expectMark: false,
			markTag:    infrastructureModeTag,
		},
	}
	e2e.Run(t, suite, suite.getSuiteOptions("check-events-full")...)
}
