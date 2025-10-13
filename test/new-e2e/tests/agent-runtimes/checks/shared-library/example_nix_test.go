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

type linuxSharedLibrarySuite struct {
	sharedLibrarySuite
}

func TestLinuxCheckImplementationSuite(t *testing.T) {
	//t.Parallel()
	suite := &linuxSharedLibrarySuite{
		sharedLibrarySuite{
			descriptor:   e2eos.UbuntuDefault,
			libraryName:  "libdatadog-agent-example.so",
			targetFolder: "/opt/datadog-agent/embedded/lib",
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions()...)
}

func (v *linuxSharedLibrarySuite) copyLibrary(sourceLibPath string) {
	// copy the lib in a tmp folder first due to restricted permissions
	v.Env().RemoteHost.CopyFile(
		sourceLibPath,
		v.Env().RemoteHost.JoinPath("/", "tmp", v.libraryName),
	)
	out := v.Env().RemoteHost.MustExecute("sudo cp " + v.Env().RemoteHost.JoinPath("/", "tmp", v.libraryName) + " " + v.Env().RemoteHost.JoinPath(v.targetFolder, v.libraryName)) // TODO: replace by a specific RemoteHost function?
	// should not output anything, otherwise it's an error
	require.Empty(v.T(), out)
}

func (v *linuxSharedLibrarySuite) removeLibrary() {
	out := v.Env().RemoteHost.MustExecute("sudo rm " + v.Env().RemoteHost.JoinPath(v.targetFolder, v.libraryName)) // TODO: replace by a specific RemoteHost function?
	// should not output anything, otherwise it's an error
	require.Empty(v.T(), out)
}

func (v *linuxSharedLibrarySuite) TestLinuxCheckExampleRun() {
	// copy the lib with the right permissions
	sourceLibPath := path.Join(".", "files", v.libraryName)
	v.copyLibrary(sourceLibPath)

	res, err := v.Env().RemoteHost.FileExists(v.Env().RemoteHost.JoinPath(v.targetFolder, v.libraryName))
	require.Nil(v.T(), err)
	require.True(v.T(), res)

	// execute the check and verify the metrics
	v.testCheckExecutionAndMetrics()

	// clean the lib after the test
	v.removeLibrary()
}
