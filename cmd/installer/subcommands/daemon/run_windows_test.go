// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
)

const (
	datadogYaml = "C:\\ProgramData\\Datadog\\datadog.yaml"
)

type daemonTestSuite struct {
	suite.Suite
	global *command.GlobalParams
}

// TestDaemonSuite runs a suite of test for the DaemonApp on Windows.
func TestDaemonSuite(t *testing.T) {
	suite.Run(t, &daemonTestSuite{
		global: &command.GlobalParams{},
	})
}

// TestRunCommand validates that our dependency graph is valid.
// This does not instantiate any component and merely validates the
// dependency graph.
func (s *daemonTestSuite) TestRunCommand() {
	s.Require().NoError(fx.ValidateApp(getFxOptions(s.global)...))
}

// TestAppStartsAndStops creates a new test app (not a daemon.windowsService)
// with our dependency graph and verify that we can start and stop it.
// This is essentially what the svc.Run code does behind the scenes.
// Note: this actually instantiates the components, so it will actually start
// the remote config service etc...
func (s *daemonTestSuite) TestAppStartsAndStops() {
	if !filesystem.FileExists(datadogYaml) {
		f, err := os.Create(datadogYaml)
		s.Require().NoError(err)
		f.Close()
		defer func() {
			os.Remove(datadogYaml)
		}()
	}
	testApp := &windowsService{
		App: fx.New(getFxOptions(s.global)...),
	}
	s.Require().NoError(testApp.Start())
	s.Require().NoError(testApp.Stop())
}

// TestAppCannotStartWithoutConfig test that without a valid config file
// the App cannot start.
func (s *daemonTestSuite) TestAppCannotStartWithoutConfig() {
	if filesystem.FileExists(datadogYaml) {
		s.Require().NoError(os.Rename(datadogYaml, datadogYaml+".bak"))
		defer func() {
			os.Rename(datadogYaml+".bak", datadogYaml)
		}()
	}

	testApp := &windowsService{
		App: fx.New(getFxOptions(s.global)...),
	}
	s.Require().Error(testApp.Start())
}
