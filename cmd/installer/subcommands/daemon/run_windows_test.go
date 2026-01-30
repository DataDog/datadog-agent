// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

type daemonTestSuite struct {
	suite.Suite
}

const (
	testCmdEnv = "DD_TEST_CMD"
)

func TestMain(m *testing.M) {
	if _, isSet := os.LookupEnv(testCmdEnv); isSet {
		commands.MockInstaller = commands.NewInstallerMock()
		cmd := &cobra.Command{
			Use: "installer [command]",
		}
		cmd.AddGroup(
			&cobra.Group{
				ID:    "installer",
				Title: "Installer Commands",
			},
			&cobra.Group{
				ID:    "apm",
				Title: "APM Commands",
			},
			&cobra.Group{
				ID:    "extension",
				Title: "Extensions Commands",
			},
		)
		cmd.AddCommand(commands.RootCommands()...)
		cmd.AddCommand(commands.UnprivilegedCommands()...)
		err := cmd.Execute()
		if err != nil {
			panic(err)
		}
		return
	}
	os.Setenv(testCmdEnv, "true")
	os.Exit(m.Run())
}

// TestDaemonSuite runs a suite of test for the DaemonApp on Windows.
func TestDaemonSuite(t *testing.T) {
	suite.Run(t, &daemonTestSuite{})
}

// TestRunCommand validates that our dependency graph is valid.
// This does not instantiate any component and merely validates the
// dependency graph.
func (s *daemonTestSuite) TestRunCommand() {
	s.Require().NoError(fx.ValidateApp(getFxOptions(&command.GlobalParams{}, &windowsService{})...))
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
	testApp := &windowsService{}
	testApp.App = fx.New(getFxOptions(
		&command.GlobalParams{ConfFilePath: tempfile.Name()},
		testApp,
	)...)
	s.Require().NoError(testApp.Start())
	s.Require().NoError(testApp.Stop())
}

// createConfigDir creates the C:\ProgramData\Datadog Installer directory with the correct permissions.
func createConfigDir(t *testing.T) {
	t.Cleanup(func() {
		// only cleanup the dir in the CI, to protect local testers while
		// this test still uses the real filesystem
		if os.Getenv("CI") != "" || os.Getenv("CI_JOB_ID") != "" {
			_ = os.RemoveAll(paths.DatadogInstallerData)
		}
	})
	err := paths.SetupInstallerDataDir()
	require.NoError(t, err)
	err = paths.IsInstallerDataDirSecure()
	require.NoError(t, err)
	err = os.MkdirAll(paths.RunPath, 0755)
	require.NoError(t, err)
}
