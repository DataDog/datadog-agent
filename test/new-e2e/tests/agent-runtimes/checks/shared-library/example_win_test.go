// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for testing a shared library check
package sharedlibrary

import (
	"path"
	"testing"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type windowsSharedLibrarySuite struct {
	sharedLibrarySuite
}

func TestWindowsCheckImplementationSuite(t *testing.T) {
	//t.Parallel()
	suite := &windowsSharedLibrarySuite{
		sharedLibrarySuite{
			descriptor:  e2eos.WindowsServerDefault,
			libraryName: "libdatadog-agent-example.dll",
			checksdPath: "C:\\ProgramData\\Datadog\\checks.d",
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions()...)
}

func (v *windowsSharedLibrarySuite) copyLibrary(sourceLibPath string) {
	v.Env().RemoteHost.CopyFile(
		sourceLibPath,
		v.Env().RemoteHost.JoinPath(v.checksdPath, v.libraryName),
	)
}

func (v *windowsSharedLibrarySuite) removeLibrary() {
	err := v.Env().RemoteHost.Remove(v.Env().RemoteHost.JoinPath(v.checksdPath, v.libraryName))
	require.Nil(v.T(), err)
}

func (v *windowsSharedLibrarySuite) TestWindowsCheckExample() {
	// copy the lib with the right permissions
	sourceLibPath := path.Join(".", "files", v.libraryName)
	v.copyLibrary(sourceLibPath)

	res, err := v.Env().RemoteHost.FileExists(v.Env().RemoteHost.JoinPath(v.checksdPath, v.libraryName))
	require.Nil(v.T(), err)
	require.True(v.T(), res)

	// execute the check and verify the metrics
	v.testCheckExecutionAndVerifyMetrics()

	// remove the lib after the test
	v.removeLibrary()
}
