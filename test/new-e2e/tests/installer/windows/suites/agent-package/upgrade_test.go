// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"

	"testing"

	"github.com/cenkalti/backoff/v4"
)

type testAgentUpgradeSuite struct {
	installerwindows.BaseSuite
}

// TestAgentUpgrades tests the usage of the Datadog installer to upgrade the Datadog Agent package.
func TestAgentUpgrades(t *testing.T) {
	e2e.Run(t, &testAgentUpgradeSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestUpgradeMSI tests manual upgrade using the Datadog Agent MSI package.
//
// The expectation is that the MSI becomes the new stable package
func (s *testAgentUpgradeSuite) TestUpgradeMSI() {
	s.setAgentConfig()

	s.installPreviousAgentVersion()
	s.assertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

	s.installCurrentAgentVersion()
	s.assertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())
}

// TestUpgradeAgentPackage tests that the daemon can upgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.assertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())

	// Assert
}

func (s *testAgentUpgradeSuite) TestRunAgentMSIAfterExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()
	s.mustStartExperimentPreviousVersion()
	s.assertSuccessfulAgentStartExperiment(s.StableAgentVersion().Version())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSILogFile("install-current-version-again.log"),
	)
	s.assertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())
}

// TestUpgradeAgentPackage tests that the daemon can downgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.mustStartExperimentPreviousVersion()
	s.assertSuccessfulAgentStartExperiment(s.StableAgentVersion().Version())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

	// Assert
}

// TestStopExperiment tests that the daemon can stop the experiment
// and that it reverts to the stable version.
func (s *testAgentUpgradeSuite) TestStopExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.assertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
}

// TestExperimentForNonExistingPackageFails tests that starting an experiment
// with a non-existing package version fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentForNonExistingPackageFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	_, err := s.Installer().StartExperiment(consts.AgentPackage, "unknown-version")
	s.Require().ErrorContains(err, "could not get package")
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}

// TestExperimentCurrentVersionFails tests that starting an experiment
// with the same version as the current one fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentCurrentVersionFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	_, err := s.startExperimentCurrentVersion()
	s.Require().ErrorContains(err, "target package already exists")
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}

func (s *testAgentUpgradeSuite) TestStopWithoutExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.Installer().StopExperiment(consts.AgentPackage)

	// Assert
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}

func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenServiceDies() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.assertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)
	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())
}

func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenTimeout() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()
	// lower timeout to 2 minute
	s.setWatchdogTimeout(2)

	// Act
	s.mustStartExperimentCurrentVersion()
	s.assertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())

	// Assert
	err := s.waitForInstallerVersion(s.StableAgentVersion().Version())
	s.Require().NoError(err)
	// wait till the services start
	err = s.waitForInstallerService("Running")
	s.Require().NoError(err)
	// verify stable version contraints
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())
}

func (s *testAgentUpgradeSuite) setWatchdogTimeout(timeout int) {
	// Set HKEY_LOCAL_MACHINE\SOFTWARE\Datadog\Datadog Agent\WatchdogTimeout to timeout
	err := windowscommon.SetRegistryDWORDValue(s.Env().RemoteHost, `HKLM:\SOFTWARE\Datadog\Datadog Agent`, "WatchdogTimeout", timeout)
	s.Require().NoError(err)
}

func (s *testAgentUpgradeSuite) installPreviousAgentVersion() {
	agentVersion := s.StableAgentVersion().Version()
	s.Require().NoError(s.Installer().Install(
		// TODO: Update when prod MSI that contains the Installer is available
		installerwindows.WithOption(installerwindows.WithURLFromPipeline("58948204")),
		installerwindows.WithMSIDevEnvOverrides("PREVIOUS_AGENT"),
		installerwindows.WithMSILogFile("install-previous-version.log"),
	))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) installCurrentAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.CurrentAgentVersion().GetNumberAndPre()

	options := []installerwindows.MsiOption{
		installerwindows.WithMSIDevEnvOverrides("CURRENT_AGENT"),
		installerwindows.WithMSILogFile("install-current-version.log"),
	}
	options = append(options, opts...)
	s.Require().NoError(s.Installer().Install(
		options...,
	))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) startExperimentWithCustomPackage(opts ...installerwindows.PackageOption) (string, error) {
	packageConfig, err := installerwindows.NewPackageConfig(opts...)
	s.Require().NoError(err)
	packageConfig, err = installerwindows.CreatePackageSourceIfLocal(s.Env().RemoteHost, packageConfig)
	s.Require().NoError(err)

	// Set catalog so daemon can find the package
	_, err = s.Installer().SetCatalog(installerwindows.Catalog{
		Packages: []installerwindows.PackageEntry{
			{
				Package: packageConfig.Name,
				Version: packageConfig.Version,
				URL:     packageConfig.URL(),
			},
		},
	})
	s.Require().NoError(err)
	return s.Installer().StartInstallerExperiment(consts.AgentPackage, packageConfig.Version)
}

func (s *testAgentUpgradeSuite) startExperimentPreviousVersion() (string, error) {
	return s.startExperimentWithCustomPackage(installerwindows.WithName(consts.AgentPackage),
		// TODO: switch to prod stable entry when available
		installerwindows.WithPipeline("58948204"),
		installerwindows.WithDevEnvOverrides("PREVIOUS_AGENT"),
	)
}

func (s *testAgentUpgradeSuite) mustStartExperimentPreviousVersion() {
	// Arrange
	agentVersion := s.StableAgentVersion().Version()

	// Act
	_, _ = s.startExperimentPreviousVersion()
	// can't check error here because the process will be killed by the MSI "files in use"
	// and experiment started in the background
	// s.Require().NoError(err)

	// Assert
	// have to wait for experiment to finish installing
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}

func (s *testAgentUpgradeSuite) startExperimentCurrentVersion() (string, error) {
	return s.startExperimentWithCustomPackage(installerwindows.WithName(consts.AgentPackage),
		// Default to using OCI package from current pipeline
		installerwindows.WithPipeline(s.Env().Environment.PipelineID()),
		installerwindows.WithDevEnvOverrides("CURRENT_AGENT"),
	)
}

func (s *testAgentUpgradeSuite) mustStartExperimentCurrentVersion() {
	// Arrange
	agentVersion := s.CurrentAgentVersion().GetNumberAndPre()

	// Act
	_, _ = s.startExperimentCurrentVersion()
	// can't check error here because the process will be killed by the MSI "files in use"
	// and experiment started in the background
	// s.Require().NoError(err)

	// Assert
	// have to wait for experiment to finish installing
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}

func (s *testAgentUpgradeSuite) setAgentConfig() {
	s.Env().RemoteHost.MkdirAll("C:\\ProgramData\\Datadog")
	s.Env().RemoteHost.WriteFile(consts.ConfigPath, []byte(`
api_key: aaaaaaaaa
remote_updates: true
`))
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentStartExperiment(version string) {
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithExperimentVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		})
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentPromoteExperiment(version string) {
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		}).
		WithExperimentVersionEqual("")
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentStopExperiment(version string) {
	// conditions are same as promote, except the stable version should be unchanged.
	// since version is an input we can reuse.
	s.assertSuccessfulAgentPromoteExperiment(version)
}

func (s *testAgentUpgradeSuite) waitForInstallerService(state string) error {
	return s.waitForInstallerServiceWithBackoff(state,
		// usually waiting after MSI runs so we have to wait awhile
		// max wait is 30*30 -> 900 seconds (15 minutes)
		backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 30))
}

func (s *testAgentUpgradeSuite) waitForInstallerServiceWithBackoff(state string, b backoff.BackOff) error {
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

func (s *testAgentUpgradeSuite) waitForInstallerVersion(version string) error {
	return s.waitForInstallerVersionWithBackoff(version,
		// usually waiting after MSI runs so we have to wait awhile
		// max wait is 30*30 -> 900 seconds (15 minutes)
		backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 30))
}

func (s *testAgentUpgradeSuite) waitForInstallerVersionWithBackoff(version string, b backoff.BackOff) error {
	return backoff.Retry(func() error {
		actual, err := s.Installer().Version()
		if err != nil {
			return err
		}
		if !strings.Contains(actual, version) {
			return fmt.Errorf("expected version %s, got %s", version, actual)
		}
		return nil
	}, b)
}
