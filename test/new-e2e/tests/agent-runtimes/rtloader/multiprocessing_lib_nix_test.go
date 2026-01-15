// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for the rtloader
package rtloader

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type linuxMultiProcessingLibSuite struct {
	baseMultiProcessingLibSuite
}

func TestLinuxMultiProcessingLibSuite(t *testing.T) {
	t.Parallel()
	suite := &linuxMultiProcessingLibSuite{baseMultiProcessingLibSuite{
		checksdPath: "/etc/datadog-agent/checks.d/multi_pid_check.py",
	}}
	e2e.Run(t, suite, suite.getSuiteOptions(os.UbuntuDefault)...)
}
