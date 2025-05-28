// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	suite := &windowsDiskCheckSuite{diskCheckSuite{descriptor: e2eos.WindowsDefault, metricCompareFraction: 0.02, metricCompareDecimals: 1}}
	e2e.Run(t, suite, suite.getSuiteOptions()...)
}
