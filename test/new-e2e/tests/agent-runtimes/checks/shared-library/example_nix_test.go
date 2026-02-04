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

var linuxDefaultPermissions = perms.NewUnixPermissions(perms.WithPermissions("0740"), perms.WithOwner("dd-agent"), perms.WithGroup("dd-agent"))

type linuxSharedLibrarySuite struct {
	sharedLibrarySuite
}

func TestLinuxSharedLibraryCheckSuite(t *testing.T) {
	t.Parallel()

	suite := &linuxSharedLibrarySuite{
		sharedLibrarySuite{
			descriptor:  e2eos.UbuntuDefault,
			checksdPath: "/tmp/datadog-agent/checks.d",
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions())
}

func (v *linuxSharedLibrarySuite) TestCheckExample() {
	v.updateEnvWithCheckConfigAndLibrary("example", exampleCheckConfig, linuxDefaultPermissions)
	v.testExampleRunAndMetrics()
}

func (v *linuxSharedLibrarySuite) TestInvalidPermissions_OthersHaveRights() {
	permissions := perms.NewUnixPermissions(perms.WithPermissions("0777"), perms.WithOwner("dd-agent"), perms.WithGroup("dd-agent"))
	v.updateEnvWithCheckConfigAndLibrary("example", exampleCheckConfig, permissions)
	v.testExampleRunExpectError("'others' have rights on it or 'group' has write permissions on it")
}

func (v *linuxSharedLibrarySuite) TestInvalidPermissions_NotAllowedOwner() {
	permissions := perms.NewUnixPermissions(perms.WithPermissions("0777"), perms.WithOwner("ubuntu"), perms.WithGroup("ubuntu"))
	v.updateEnvWithCheckConfigAndLibrary("example", exampleCheckConfig, permissions)
	v.testExampleRunExpectError("file owner is neither `root`, `dd-agent` or current user")
}
