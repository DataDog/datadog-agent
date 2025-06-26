// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checknetwork contains tests for the network check
package checknetwork

import (
	"testing"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type windowsNetworkCheckSuite struct {
	networkCheckSuite
}

func TestWindowsNetworkSuite(t *testing.T) {
	t.Parallel()
	suite := &windowsNetworkCheckSuite{
		networkCheckSuite{
			descriptor:            e2eos.WindowsDefault,
			metricCompareFraction: 0.1,
			metricCompareDecimals: 1,
		},
	}
	e2e.Run(t, suite, suite.getSuiteOptions()...)
}
