// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	agentVersion "github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	"os"
)

// BaseSuite the base suite for all installer tests on Windows (install script, MSI, exe etc...).
// To run the test suites locally, pick a pipeline and define the following environment variables:
// E2E_PIPELINE_ID: the ID of the pipeline
// CURRENT_AGENT_VERSION: pull it from one of the jobs that builds the Agent
// STABLE_INSTALLER_VERSION_PACKAGE: use `crane ls public.ecr.aws/datadog/installer-package | sort | tail -n 2 | head -n 1`
// STABLE_AGENT_VERSION_PACKAGE: use `crane ls public.ecr.aws/datadog/agent-package | sort | tail -n 2 | head -n 1`
// or pick any other version from that registry.
//
// For example:
//
//	CI_COMMIT_SHA=ac2acaffab7b039f8c2524df8ae82f9f5fd04d5d;
//	E2E_PIPELINE_ID=40537701;
//	CURRENT_AGENT_VERSION=7.57.0-devel+git.370.d429ae3;
//	STABLE_INSTALLER_VERSION_PACKAGE=7.56.0-installer-0.4.6-1-1
//	STABLE_AGENT_VERSION_PACKAGE=7.55.2-1
type BaseSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	installer              *DatadogInstaller
	installScript          *DatadogInstallScript
	currentAgentVersion    agentVersion.Version
	stableInstallerVersion PackageVersion
	stableAgentVersion     PackageVersion
}

// Installer The Datadog Installer for testing.
func (s *BaseSuite) Installer() *DatadogInstaller {
	return s.installer
}

// InstallScript The Datadog Install script for testing.
func (s *BaseSuite) InstallScript() *DatadogInstallScript { return s.installScript }

// Require instantiates a suiteAssertions for the current suite.
// This allows writing assertions in a "natural" way, i.e.:
//
//	suite.Require().HasAService(...).WithUserSid(...)
//
// Ideally this suite assertion would exist at a higher level of abstraction
// so that it could be shared by multiple suites, but for now it exists only
// on the Windows Datadog installer `BaseSuite` object.
func (s *BaseSuite) Require() *suiteasserts.SuiteAssertions {
	return suiteasserts.New(s.BaseSuite.Require(), s)
}

// CurrentAgentVersion the version of the Agent in the current pipeline
func (s *BaseSuite) CurrentAgentVersion() *agentVersion.Version {
	return &s.currentAgentVersion
}

// StableInstallerVersion the version of the last published stable installer
func (s *BaseSuite) StableInstallerVersion() PackageVersion {
	return s.stableInstallerVersion
}

// StableAgentVersion the version of the last published stable agent
func (s *BaseSuite) StableAgentVersion() PackageVersion {
	return s.stableAgentVersion
}

// SetupSuite checks that the environment variables are correctly setup for the test
func (s *BaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// TODO:FA-779
	if s.Env().Environment.PipelineID() == "" && os.Getenv("DD_INSTALLER_MSI_URL") == "" {
		s.FailNow("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
	}

	var err error
	s.currentAgentVersion, err = agentVersion.New(os.Getenv("CURRENT_AGENT_VERSION"), "")
	s.Require().NoError(err, "Agent version was in an incorrect format")

	s.stableInstallerVersion = newVersionFromPackageVersion(os.Getenv("STABLE_INSTALLER_VERSION_PACKAGE"))
	if s.stableInstallerVersion.PackageVersion() == "" {
		s.FailNow("STABLE_INSTALLER_VERSION_PACKAGE was not set")
	}

	s.stableAgentVersion = newVersionFromPackageVersion(os.Getenv("STABLE_AGENT_VERSION_PACKAGE"))
	if s.stableAgentVersion.PackageVersion() == "" {
		s.FailNow("STABLE_AGENT_VERSION_PACKAGE was not set")
	}
}

// BeforeTest creates a new Datadog Installer and sets the output logs directory for each tests
func (s *BaseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	s.installer = NewDatadogInstaller(s.Env(), s.SessionOutputDir())
	s.installScript = NewDatadogInstallScript(s.Env())
}
