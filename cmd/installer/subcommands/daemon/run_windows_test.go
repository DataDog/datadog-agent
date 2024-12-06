// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"
	"golang.org/x/sys/windows"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
)

type daemonTestSuite struct {
	suite.Suite
}

// TestDaemonSuite runs a suite of test for the DaemonApp on Windows.
func TestDaemonSuite(t *testing.T) {
	suite.Run(t, &daemonTestSuite{})
}

// TestRunCommand validates that our dependency graph is valid.
// This does not instantiate any component and merely validates the
// dependency graph.
func (s *daemonTestSuite) TestRunCommand() {
	s.Require().NoError(fx.ValidateApp(getFxOptions(&command.GlobalParams{})...))
}

// TestAppStartsAndStops creates a new app with our dependency graph and verify that we can start and stop it.
// This is essentially what the svc.Run code does behind the scenes.
// Note: this actually instantiates the components, so it will actually start
// the remote config service etc...
func (s *daemonTestSuite) TestAppStartsAndStops() {
	// TODO: This test currently tries to start the daemon using the system paths
	createConfigDir(s.T())
	tempfile, err := os.CreateTemp("", "test-*.yaml")
	require.NoError(s.T(), err, "failed to create temporary file")
	defer os.Remove(tempfile.Name())
	testApp := &windowsService{
		App: fx.New(getFxOptions(&command.GlobalParams{
			ConfFilePath: tempfile.Name(),
		})...),
	}
	s.Require().NoError(testApp.Start())
	s.Require().NoError(testApp.Stop())
}

// createConfigDir creates the C:\ProgramData\Datadog Installer directory with the correct permissions.
func createConfigDir(t *testing.T) {
	t.Cleanup(func() {
		// only cleanup the dir in the CI, to protect local testers while
		// this test still uses the real filesystem
		if os.Getenv("CI") != "" || os.Getenv("CI_JOB_ID") != "" {
			_ = os.RemoveAll("C:\\ProgramData\\Datadog Installer")
		}
	})
	err := os.MkdirAll("C:\\ProgramData\\Datadog Installer", 0)
	require.NoError(t, err)
	owner, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	require.NoError(t, err)
	err = windows.SetNamedSecurityInfo("C:\\ProgramData\\Datadog Installer", windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION, owner, nil, nil, nil)
	require.NoError(t, err)
}
