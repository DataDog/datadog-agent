// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	"os"
	"strings"
)

// BaseInstallerSuite the base suite for all installer tests on Windows.
// To run the test suites locally, pick a pipeline and define the following environment variables:
// E2E_PIPELINE_ID: the ID of the pipeline
// CURRENT_AGENT_VERSION: pull it from one of the jobs that builds the Agent
// STABLE_INSTALLER_VERSION_PACKAGE: use `crane ls public.ecr.aws/datadog/installer-package | sort | tail -n 2 | head -n 1` '
// or pick any other version from that registry.
//
// For example:
//
//	E2E_PIPELINE_ID=40537701;
//	CURRENT_AGENT_VERSION=7.57.0-devel+git.370.d429ae3;
//	STABLE_INSTALLER_VERSION_PACKAGE=7.56.0-installer-0.4.6-1-1
type BaseInstallerSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	installer                     *DatadogInstaller
	currentAgentVersion           string
	stableInstallerVersionPackage string
	stableInstallerVersion        string
}

// Installer the Datadog Installer for testing.
func (s *BaseInstallerSuite) Installer() *DatadogInstaller {
	return s.installer
}

// CurrentAgentVersion the version of the Agent in the current pipeline
func (s *BaseInstallerSuite) CurrentAgentVersion() string {
	return s.currentAgentVersion
}

// StableInstallerVersion the version of the last published stable installer
func (s *BaseInstallerSuite) StableInstallerVersion() string {
	return s.stableInstallerVersion
}

// StableInstallerVersionPackage same as StableInstallerVersion but with the suffix `-1`
func (s *BaseInstallerSuite) StableInstallerVersionPackage() string {
	return s.stableInstallerVersionPackage
}

// SetupSuite checks that the environment variables are correctly setup for the test
func (s *BaseInstallerSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// TODO:FA-779
	if s.Env().AwsEnvironment.PipelineID() == "" {
		s.FailNow("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
	}

	s.currentAgentVersion = os.Getenv("CURRENT_AGENT_VERSION")
	if s.currentAgentVersion == "" {
		s.FailNow("Set CURRENT_AGENT_VERSION")
	}

	s.stableInstallerVersionPackage = os.Getenv("STABLE_INSTALLER_VERSION_PACKAGE")
	if s.stableInstallerVersionPackage == "" {
		s.FailNow("Set STABLE_INSTALLER_VERSION_PACKAGE")
	}
	s.stableInstallerVersion = strings.TrimSuffix(s.stableInstallerVersionPackage, "-1")
}

// BeforeTest creates a new Datadog Installer and sets the output logs directory for each tests
func (s *BaseInstallerSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), s.T())
	s.Require().NoError(err, "should get output dir")
	s.T().Logf("Output dir: %s", outputDir)
	s.installer = NewDatadogInstaller(s.Env(), fmt.Sprintf("%s/install.log", outputDir))
}

// Require instantiates a suiteAssertions for the current suite.
// This allows writing assertions in a "natural" way, i.e.:
//
//	suite.Require().HasAService(...).WithUserSid(...)
//
// Ideally this suite assertion would exist at a higher level of abstraction
// so that it could be shared by multiple suites, but for now it exists only
// on the Windows Datadog installer `BaseInstallerSuite` object.
func (s *BaseInstallerSuite) Require() *suiteasserts.SuiteAssertions {
	return suiteasserts.New(s.BaseSuite.Require(), s)
}
