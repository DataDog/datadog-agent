// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for testing a shared library check
package sharedlibrary

import (
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type windowsSharedLibrarySuite struct {
	sharedLibrarySuite
}

func TestWindowsCheckImplementationSuite(t *testing.T) {
	t.Parallel()
	suite := &windowsSharedLibrarySuite{
		sharedLibrarySuite{
			descriptor:  e2eos.WindowsServerDefault,
			libraryName: "libdatadog-agent-example.dll",
			checksdPath: "C:\\Temp\\Datadog\\checks.d",
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions()...)
}

func (v *windowsSharedLibrarySuite) TestWindowsCheckExample() {
	v.testCheckExample()
}
