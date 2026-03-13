// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for testing a shared library check
package sharedlibrary

import (
	"testing"

	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

var windowsDefaultPermissions = perms.NewWindowsPermissions(
	perms.WithIcaclsCommand(`/grant "ddagentuser:(D,WDAC,RX,RA)" /grant "Administrators:(RX)" /grant "SYSTEM:(RX)"`),
	perms.WithDisableInheritance(),
)

type windowsSharedLibrarySuite struct {
	sharedLibrarySuite
}

func TestWindowsSharedLibraryCheckSuite(t *testing.T) {
	t.Parallel()

	suite := &windowsSharedLibrarySuite{
		sharedLibrarySuite{
			descriptor:  e2eos.WindowsServerDefault,
			checksdPath: "C:/Temp/Datadog/checks.d",
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions())
}

func (v *windowsSharedLibrarySuite) TestCheckExample() {
	v.updateEnvWithCheckConfigAndSharedLibrary("example", checkMinimalConfig, windowsDefaultPermissions)
	v.testCheckExampleExecutionAndMetrics()
}

func (v *windowsSharedLibrarySuite) TestCheckWithoutRunSymbol() {
	v.updateEnvWithCheckConfigAndSharedLibrary("no-run-symbol", checkMinimalConfig, windowsDefaultPermissions)
	v.testCheckWithoutRunSymbolExecutionError()
}
