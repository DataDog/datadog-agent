// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"os"
)

// BaseSuite the base suite for all installer tests on Windows (install script, MSI, exe etc...).
// To run the test suites locally, pick a pipeline and define the following environment variables:
// E2E_PIPELINE_ID: the ID of the pipeline
// CURRENT_AGENT_VERSION: pull it from one of the jobs that builds the Agent
// STABLE_AGENT_VERSION_PACKAGE: use `crane ls public.ecr.aws/datadog/agent-package | sort | tail -n 2 | head -n 1`
// or pick any other version from that registry.
//
// For example:
//
//	CI_COMMIT_SHA=ac2acaffab7b039f8c2524df8ae82f9f5fd04d5d;
//	E2E_PIPELINE_ID=40537701;
//	CURRENT_AGENT_VERSION=7.57.0-devel+git.370.d429ae3;
//	STABLE_AGENT_VERSION_PACKAGE=7.55.2-1
type BaseSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	installer          *DatadogInstaller
	installScriptImpl  InstallScriptRunner
	currentAgent       *AgentVersionManager
	stableAgent        *AgentVersionManager
	CreateCurrentAgent func() (*AgentVersionManager, error)
	CreateStableAgent  func() (*AgentVersionManager, error)
}

// Installer The Datadog Installer for testing.
func (s *BaseSuite) Installer() *DatadogInstaller {
	return s.installer
}

// InstallScript returns the installer implementation.
// Override this method in your test suite to use a different implementation.
func (s *BaseSuite) InstallScript() InstallScriptRunner {
	return s.installScriptImpl
}

// SetInstallScriptImpl sets a custom installer implementation.
// Use this in your test suite's SetupSuite to override the default implementation.
func (s *BaseSuite) SetInstallScriptImpl(impl InstallScriptRunner) {
	s.installScriptImpl = impl
}

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
func (s *BaseSuite) CurrentAgentVersion() *AgentVersionManager {
	return s.currentAgent
}

// StableAgentVersion the version of the last published stable agent
func (s *BaseSuite) StableAgentVersion() *AgentVersionManager {
	return s.stableAgent
}

// SetupSuite checks that the environment variables are correctly setup for the test
func (s *BaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	// The below current and stable artifacts can be configured with environment variables.
	// See doc.go for more information.
	// TODO: not every test needs every artifact, it might be nice to have a way to opt-in to specific artifacts
	//       which would let us create better "required but not set" messages.
	s.createCurrentAgent()
	s.T().Logf("current agent version: %s", s.CurrentAgentVersion())
	s.createStableAgent()
	s.T().Logf("stable agent version: %s", s.StableAgentVersion())
}

// createCurrentAgent sets the current agent version for the test suite.
//
// By default, the current agent is the current pipeline, but tests can
// override this by setting the CreateCurrentAgent function.
//
// For testing, the version and artifacts can be overridden via environment variables, see
// doc.go for more information.
func (s *BaseSuite) createCurrentAgent() {
	if s.CreateCurrentAgent != nil {
		agent, err := s.CreateCurrentAgent()
		s.Require().NoError(err, "failed to create current agent")
		s.currentAgent = agent
		return
	}
	// else, use the defaults (current pipeline)

	// Get current version OCI package
	currentOCI, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithPipeline(s.Env().Environment.PipelineID()),
		WithDevEnvOverrides("CURRENT_AGENT"),
	)
	s.Require().NoError(err, "failed to lookup OCI package for current agent version")

	// Get current version MSI package
	currentMSI, err := windowsagent.NewPackage(
		windowsagent.WithURLFromPipeline(s.Env().Environment.PipelineID()),
		windowsagent.WithDevEnvOverrides("CURRENT_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup MSI for current agent version")
	s.Require().NotEmpty(currentMSI.URL, "Agent MSI URL is required but not set, set E2E_PIPELINE_ID or CURRENT_AGENT devenv overrides")

	// Setup current Agent artifacts
	currentVersion, currentPackageVersion := s.getAgentVersionVars("CURRENT_AGENT")
	s.currentAgent, err = NewAgentVersionManager(
		currentVersion,
		currentPackageVersion,
		currentOCI,
		currentMSI,
	)
	s.Require().NoError(err, "Current agent version was in an incorrect format")
}

// createStableAgent sets the stable agent version for the test suite.
//
// By default, the stable agent is the last stable release, but tests can
// override this by setting the CreateStableAgent function.
//
// For testing, the version and artifacts can be overridden via environment variables, see
// doc.go for more information.
func (s *BaseSuite) createStableAgent() {
	if s.CreateStableAgent != nil {
		agent, err := s.CreateStableAgent()
		s.Require().NoError(err, "failed to create stable agent")
		s.stableAgent = agent
		return
	}
	// else, use the defaults (last stable release)

	agentVersion := "7.66.0-devel"
	agentVersionPackage := "7.66.0-devel.git.488.1ddea94.pipeline.62296915-1"
	// Allow override of version and version package via environment variables
	if val := os.Getenv("STABLE_AGENT_VERSION"); val != "" {
		agentVersion = val
	}
	if val := os.Getenv("STABLE_AGENT_VERSION_PACKAGE"); val != "" {
		agentVersionPackage = val
	}

	// Get previous version OCI package
	previousOCI, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		// TODO: update to last stable when there is one
		WithVersion(agentVersionPackage),
		WithRegistry("install.datad0g.com"),
		WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup OCI package for previous agent version")

	// Get previous version MSI package
	previousMSI, err := windowsagent.NewPackage(
		// TODO: update to last stable when there is one
		windowsagent.WithVersion(agentVersionPackage),
		windowsagent.WithURL("https://s3.amazonaws.com/dd-agent-mstesting/builds/dev/ddagent-cli-7.66.0-devel.git.488.1ddea94.pipeline.62296915.msi"),
		windowsagent.WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup MSI for previous agent version")

	// Setup previous Agent artifacts
	s.stableAgent, err = NewAgentVersionManager(
		agentVersion,
		agentVersionPackage,
		previousOCI,
		previousMSI,
	)
	s.Require().NoError(err, "Stable agent version was in an incorrect format")
}

// getAgentVersionVars retrieves the agent version and package version from environment variables
//
// example: CURRENT_AGENT_VERSION and CURRENT_AGENT_VERSION_PACKAGE
//
// see doc.go for more information
func (s *BaseSuite) getAgentVersionVars(prefix string) (string, string) {
	versionVar := fmt.Sprintf("%s_VERSION", prefix)
	versionPackageVar := fmt.Sprintf("%s_VERSION_PACKAGE", prefix)

	// Agent version
	version := os.Getenv(versionVar)
	s.Require().NotEmpty(versionVar, "%s is required but not set", versionVar)

	// Package version
	versionPackage := os.Getenv(versionPackageVar)
	if versionPackage == "" && os.Getenv("CI") == "" {
		// locally, the version package can be the same as the version
		versionPackage = version
	} else {
		// The CI is expected to configure this
		s.Require().NotEmpty(versionPackage, "%s is required but not set", versionPackageVar)
	}

	return version, versionPackage
}

// BeforeTest creates a new Datadog Installer and sets the output logs directory for each tests
func (s *BaseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// Create a new subdir per test since these suites often have multiple tests
	testPart := common.SanitizeDirectoryName(testName)
	outputDir := filepath.Join(s.SessionOutputDir(), testPart)
	s.Require().NoError(os.MkdirAll(outputDir, 0755))

	s.installer = NewDatadogInstaller(s.Env(), s.CurrentAgentVersion().MSIPackage().URL, outputDir)
	s.installScriptImpl = NewDatadogInstallScript(s.Env())
}

// SetCatalogWithCustomPackage sets the catalog with a custom package
// and returns the package config created from the opts.
func (s *BaseSuite) SetCatalogWithCustomPackage(opts ...PackageOption) (TestPackageConfig, error) {
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
	return packageConfig, err
}

func (s *BaseSuite) startExperimentWithCustomPackage(opts ...PackageOption) (string, error) {
	packageConfig, err := s.SetCatalogWithCustomPackage(opts...)
	s.Require().NoError(err)
	return s.Installer().StartExperiment(consts.AgentPackage, packageConfig.Version)
}

func (s *BaseSuite) startExperimentPreviousVersion() (string, error) {
	return s.startExperimentWithCustomPackage(WithName(consts.AgentPackage),
		WithPackage(s.StableAgentVersion().OCIPackage()),
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
	agentVersion := s.CurrentAgentVersion().Version()

	// Act
	_, _ = s.StartExperimentCurrentVersion()
	// can't check error here because the process will be killed by the MSI "files in use"
	// and experiment started in the background
	// s.Require().NoError(err)

	// Assert
	// have to wait for experiment to finish installing
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	// sanity check: make sure we did indeed install the current version
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
		}).
		HasARunningDatadogAgentService()
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
		WithExperimentVersionEqual("").
		HasARunningDatadogAgentService()
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
