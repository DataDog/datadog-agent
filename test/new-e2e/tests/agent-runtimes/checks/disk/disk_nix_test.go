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

type linuxDiskCheckSuite struct {
	diskCheckSuite
}

func TestLinuxDiskSuite(t *testing.T) {
	t.Parallel()
	suite := &linuxDiskCheckSuite{
		diskCheckSuite{
			descriptor:                  e2eos.UbuntuDefault,
			metricCompareFraction:       0.02,
			metricCompareDecimals:       1,
			excludedFromValueComparison: []string{},
		},
	}
	e2e.Run(t, suite, suite.getSuiteOptions()...)
}
