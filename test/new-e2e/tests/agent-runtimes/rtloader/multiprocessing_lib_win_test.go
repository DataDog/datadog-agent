// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for the rtloader
package rtloader

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type windowsMultiProcessingLibSuite struct {
	baseMultiProcessingLibSuite
}

func TestWindowsMultiProcessingLibSuite(t *testing.T) {
	t.Parallel()
	suite := &windowsMultiProcessingLibSuite{baseMultiProcessingLibSuite{
		confdPath:   "C:/ProgramData/Datadog/conf.d/multi_file_check.yaml",
		checksdPath: "C:/ProgramData/Datadog/checks.d/multi_file_check.py",
		tempDir:     "C:/Users/ddagentuser/AppData/Local/Temp/multi_file_check",
	}}
	e2e.Run(t, suite, suite.getSuiteOptions(os.WindowsDefault)...)
}
