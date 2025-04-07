// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains code for the E2E tests for the Datadog installer on Windows
package installer

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff"

	agentVersion "github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"

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
	installer           *DatadogInstaller
	installScript       *DatadogInstallScript
	currentAgentVersion agentVersion.Version
	stableAgentVersion  PackageVersion
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

// StableAgentVersion the version of the last published stable agent
func (s *BaseSuite) StableAgentVersion() PackageVersion {
	return s.stableAgentVersion
}

// SetupSuite checks that the environment variables are correctly setup for the test
func (s *BaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	// TODO:FA-779
	if s.Env().Environment.PipelineID() == "" && os.Getenv("DD_INSTALLER_MSI_URL") == "" {
		s.FailNow("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
	}

	var err error
	s.currentAgentVersion, err = agentVersion.New(os.Getenv("CURRENT_AGENT_VERSION"), "")
	s.Require().NoError(err, "Agent version was in an incorrect format")

	s.stableAgentVersion = NewVersionFromPackageVersion(os.Getenv("STABLE_AGENT_VERSION_PACKAGE"))
	if s.stableAgentVersion.PackageVersion() == "" {
		s.FailNow("STABLE_AGENT_VERSION_PACKAGE was not set")
	}
}

// BeforeTest creates a new Datadog Installer and sets the output logs directory for each tests
func (s *BaseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// Create a new subdir per test since these suites often have multiple tests
	testPart := common.SanitizeDirectoryName(testName)
	outputDir := filepath.Join(s.SessionOutputDir(), testPart)
	s.Require().NoError(os.MkdirAll(outputDir, 0755))

	s.installer = NewDatadogInstaller(s.Env(), outputDir)
	s.installScript = NewDatadogInstallScript(s.Env())
}

func (s *BaseSuite) startExperimentWithCustomPackage(opts ...PackageOption) (string, error) {
	packageConfig, err := NewPackageConfig(opts...)
	s.Require().NoError(err)
	packageConfig, err = CreatePackageSourceIfLocal(s.Env().RemoteHost, packageConfig)
	s.Require().NoError(err)

	// Set catalog so daemon can find the package
	_, err = s.Installer().SetCatalog(Catalog{
		Packages: []PackageEntry{
			{
				Package: packageConfig.Name,
				Version: packageConfig.Version,
				URL:     packageConfig.URL(),
			},
		},
	})
	s.Require().NoError(err)
	return s.Installer().StartExperiment(consts.AgentPackage, packageConfig.Version)
}

func (s *BaseSuite) startExperimentPreviousVersion() (string, error) {
	return s.startExperimentWithCustomPackage(WithName(consts.AgentPackage),
		// TODO: switch to prod stable entry when available
		WithPipeline("58948204"),
		WithDevEnvOverrides("PREVIOUS_AGENT"),
	)
}

// MustStartExperimentPreviousVersion starts an experiment with the previous version of the Agent
func (s *BaseSuite) MustStartExperimentPreviousVersion() {
	// Arrange
	agentVersion := s.StableAgentVersion().Version()

	// Act
	_, _ = s.startExperimentPreviousVersion()
	// can't check error here because the process will be killed by the MSI "files in use"
	// and experiment started in the background
	// s.Require().NoError(err)

	// Assert
	// have to wait for experiment to finish installing
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}

// StartExperimentCurrentVersion starts an experiment of current agent version
func (s *BaseSuite) StartExperimentCurrentVersion() (string, error) {
	return s.startExperimentWithCustomPackage(WithName(consts.AgentPackage),
		// Default to using OCI package from current pipeline
		WithPipeline(s.Env().Environment.PipelineID()),
		WithDevEnvOverrides("CURRENT_AGENT"),
	)
}

// MustStartExperimentCurrentVersion start an experiment with current version of the Agent
func (s *BaseSuite) MustStartExperimentCurrentVersion() {
	// Arrange
	agentVersion := s.CurrentAgentVersion().GetNumberAndPre()

	// Act
	_, _ = s.StartExperimentCurrentVersion()
	// can't check error here because the process will be killed by the MSI "files in use"
	// and experiment started in the background
	// s.Require().NoError(err)

	// Assert
	// have to wait for experiment to finish installing
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}

// AssertSuccessfulAgentStartExperiment that experiment started successfully
func (s *BaseSuite) AssertSuccessfulAgentStartExperiment(version string) {
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithExperimentVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		})
}

// AssertSuccessfulAgentPromoteExperiment that experiment was promoted successfully
func (s *BaseSuite) AssertSuccessfulAgentPromoteExperiment(version string) {
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		}).
		WithExperimentVersionEqual("")
}

// WaitForInstallerService waits for installer service to be expected state
func (s *BaseSuite) WaitForInstallerService(state string) error {
	return s.waitForInstallerServiceWithBackoff(state,
		// usually waiting after MSI runs so we have to wait awhile
		// max wait is 30*30 -> 900 seconds (15 minutes)
		backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 30))
}

func (s *BaseSuite) waitForInstallerServiceWithBackoff(state string, b backoff.BackOff) error {
	return backoff.Retry(func() error {
		out, err := windowscommon.GetServiceStatus(s.Env().RemoteHost, consts.ServiceName)
		if err != nil {
			return err
		}
		if !strings.Contains(out, state) {
			return fmt.Errorf("expected state %s, got %s", state, out)
		}
		return nil
	}, b)
}
