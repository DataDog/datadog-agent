// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkdisk contains tests for the disk check
package checkdisk

import (
	"testing"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type windowsDiskCheckSuite struct {
	diskCheckSuite
}

func TestWindowsDiskSuite(t *testing.T) {
	t.Parallel()
	suite := &windowsDiskCheckSuite{
		diskCheckSuite{
			Descriptor:            e2eos.WindowsDefault,
			MetricCompareFraction: 0.02,
			MetricCompareDecimals: 1,
		},
	}
	e2e.Run(t, suite, suite.getSuiteOptions()...)
}
