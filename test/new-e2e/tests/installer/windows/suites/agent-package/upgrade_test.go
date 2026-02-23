// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/cenkalti/backoff/v5"
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
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	s.installCurrentAgentVersion()
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

// TestUpgradeAgentPackage tests that the daemon can upgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	windowsagent.TestAgentHasNoWorldWritablePaths(s.T(), s.Env().RemoteHost)
}

// TestUpgradeAgentPackageOCIBootstrap tests the upgrade workflow using the OCI bootstrap path.
// It uses InstallerBootstrapMode=OCI to force the OCI path, ensuring the test fails if the
// installer layer is missing from the OCI package.
//
// This test validates that:
// 1. The current pipeline OCI package contains the installer layer
// 2. The OCI bootstrap code path extracts and uses the installer correctly
//
// If this test fails, check that the OCI package build includes --installer flag.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackageOCIBootstrap() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()
	s.setInstallerBootstrapMode("OCI")

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

// TestUpgradeAgentPackageMSIBootstrap tests the upgrade workflow using the MSI fallback bootstrap path.
// It uses InstallerBootstrapMode=MSI to force the MSI path, validating backward compatibility
// with older OCI packages (< 7.70) that don't have a dedicated installer layer.
//
// IMPORTANT: Do not remove this test without ensuring backward compatibility is still tested.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackageMSIBootstrap() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()
	s.setInstallerBootstrapMode("MSI")

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

// TestUpgradeAgentPackageWithAltDir tests that an Agent installed with the MSI
// and custom paths maintains those paths when remotely upgraded
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackageWithAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`)
}

// TestUpgradeAgentPackageFromExeWithAltDir tests that an Agent installed with the .exe
// and custom paths maintains those paths when remotely upgraded
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackageFromExeWithAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	// TODO: build into AgentVersionManager?
	url := fmt.Sprintf("https://s3.amazonaws.com/dd-agent/datadog-installer-%s-x86_64.exe", s.StableAgentVersion().PackageVersion())
	installExe := installerwindows.NewDatadogInstallExe(s.Env().RemoteHost)
	output, err := installExe.Run(
		installerwindows.WithInstallerURL(url),
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_PROJECTLOCATION":          altInstallPath,
			"DD_APPLICATIONDATADIRECTORY": altConfigRoot,
			// TODO: these need to be overridden here so they're not overriden by installer.InstallScriptEnv
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT": "",
			"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":        s.StableAgentVersion().OCIPackage().Registry,
		}),
		installerwindows.WithInstallScriptDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoErrorf(err, "failed to install stable agent via exe: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err = s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`)
}

// TestUpgradeAgentPackageAfterRollback tests that upgrade works after an initial upgrade failed.
//
// This is a regression test for WINA-1469, where the Agent account password and
// password from the LSA did not match after rollback to a version before LSA support was added.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackageAfterRollback() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())

	// stop experiment to trigger rollback
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())

	// Try upgrade again
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	windowsagent.TestAgentHasNoWorldWritablePaths(s.T(), s.Env().RemoteHost)
}

// TestRunAgentMSIAfterExperiment tests that the Agent can be upgraded after
// an experiment has been run.
//
// Since the MSI removes the `packages/datadog-agent` directory, we wanted to be sure
// that MSIs Source Resiliency wouldn't have the MSI in the stable dir, which may be
// run during RemoveExistingProducts, locked and unable to be removed.
func (s *testAgentUpgradeSuite) TestRunAgentMSIAfterExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()
	s.MustStartExperimentPreviousVersion()
	s.AssertSuccessfulAgentStartExperiment(s.StableAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSILogFile("install-current-version-again.log"),
	)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

// TestUpgradeAgentPackage tests that the daemon can downgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.MustStartExperimentPreviousVersion()
	s.AssertSuccessfulAgentStartExperiment(s.StableAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// Assert
}

// TestStopExperiment tests that the daemon can stop the experiment
// and that it reverts to the stable version.
func (s *testAgentUpgradeSuite) TestStopExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	windowsagent.TestAgentHasNoWorldWritablePaths(s.T(), s.Env().RemoteHost)
}

// TestExperimentForNonExistingPackageFails tests that starting an experiment
// with a non-existing package version fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentForNonExistingPackageFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()
	s.Require().NoError(s.WaitForInstallerService("Running"))

	// Act
	_, err := s.Installer().StartExperiment(consts.AgentPackage, "unknown-version")
	s.Require().ErrorContains(err, "could not get package")

	// Assert
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

// TestExperimentCurrentVersionFails tests that starting an experiment
// with the same version as the current one fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentCurrentVersionFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()
	s.Require().NoError(s.WaitForInstallerService("Running"))

	// Act
	_, err := s.StartExperimentCurrentVersion()
	s.Require().ErrorContains(err, "cannot set new experiment to the same version as stable")

	// Assert
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

func (s *testAgentUpgradeSuite) TestStopWithoutExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

// TestRevertsExperimentWhenServiceDies tests that the watchdog will revert
// to stable version when the service dies.
func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenServiceDies() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)
	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})
}

// TestRevertsExperimentWhenServiceDies tests that the watchdog will revert
// to stable version when the timeout expires.
func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenTimeout() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()
	// lower timeout to 2 minute
	s.setWatchdogTimeout(2)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	err := s.waitForInstallerVersion(s.StableAgentVersion().Version())
	s.Require().NoError(err)
	// wait till the services start
	err = s.WaitForInstallerService("Running")
	s.Require().NoError(err)
	// verify stable version contraints
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})
}

// TestExperimentMSIRollbackMaintainsCustomUserAndAltDir tests that the
// stable version is reinstalled with the custom user and alt dir when an experiment MSI rolls back.
// This is a regression test for WINA-1504, where remove-experiment subcommand used the wrong
// paths and failed to restore the stable version.
func (s *testAgentUpgradeSuite) TestExperimentMSIRollbackMaintainsCustomUserAndAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	agentUser := "customuser"
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installPreviousAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)
	s.setExperimentMSIArgs([]string{"WIXFAILWHENDEFERRED=1"})

	// Act
	s.WaitForDaemonToStop(func() {
		_, err := s.StartExperimentCurrentVersion()
		s.Require().NoError(err, "daemon should stop cleanly")
		// This returns while the upgrade is still running, so we need to wait for the service to stop
		// We can't use WaitForInstallerService here because it can be racy with MSI rollback,
		// the service could stop and then restart before we check the status again.
	}, backoff.WithBackOff(backoff.NewConstantBackOff(5*time.Second)), backoff.WithMaxTries(100))

	// wait for upgrade to restart the service
	// this is racy, we'll either catch the new service running briefly before MSI rollback
	// triggers, or we'll catch the previous service running after MSI rollback completes
	// The next set of checks quiesce the race.
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	// Now that the service is running, we know that the stable version has been removed,
	// so we can wait for the stable version to be placed on disk once again via MSI rollback
	err = s.waitForInstallerVersion(s.StableAgentVersion().Version())
	s.Require().NoError(err)
	// and wait again to ensure the stable service is running
	err = s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	// Assert

	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	// backend will send stop experiment to the daemon
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})

	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`).
		HasAService("datadogagent").
		WithIdentity(identity)
}

// TestExperimentMSIRollbackMaintainsCustomUserAndAltDir tests that the
// stable version is reinstalled with the custom user and alt dir when an experiment MSI rolls back.
// This is a regression test for WINA-1504, where remove-experiment subcommand used the wrong
// paths and failed to restore the stable version.
func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenServiceDiesMaintainsCustomUserAndAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	agentUser := "customuser"
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installPreviousAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)
	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	// backend will send stop experiment to the daemon
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})

	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`).
		HasAService("datadogagent").
		WithIdentity(identity)
}

func (s *testAgentUpgradeSuite) getNewHostname() string {
	// add a random string to the hostname
	randomString := uuid.New().String()

	// truncate to 15 characters
	// this is to deal with the fact that the hostname is limited to 15 characters
	return randomString[0:15]
}

// TestUpgradeWithHostNameChange tests that the agent can be upgraded when the host name changes.
func (s *testAgentUpgradeSuite) TestUpgradeWithHostNameChange() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	// change the host name
	newHostname := s.getNewHostname()
	// this also deals with retries as it will always make it a new hostname
	err := windowscommon.RenameComputer(s.Env().RemoteHost, newHostname)
	s.Require().NoError(err)

	// Assert
	// start experiment
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err = s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

}

// TestUpgradeWithAgentUser tests that the agent user is preserved across remote upgrades.
func (s *testAgentUpgradeSuite) TestUpgradeWithAgentUser() {
	// Arrange
	s.setAgentConfig()
	agentUser := "customuser"
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installPreviousAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
	)
	// sanity check that the agent is running as the custom user
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		HasAService("datadogagent").
		WithIdentity(identity)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err = s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		HasAService("datadogagent").
		WithIdentity(identity)
}

// TestUpgradeWithLocalSystemUser tests that the agent user is preserved across remote upgrades.
// Also serves as a regression test for WINA-1742, since it will upgrade with DDAGENTUSER_NAME="NT AUTHORITY\SYSTEM"
// which contains a space.
func (s *testAgentUpgradeSuite) TestUpgradeWithLocalSystemUser() {
	// Arrange
	s.setAgentConfig()
	agentUserInput := "LocalSystem"
	agentDomainExpected := "NT AUTHORITY"
	agentUserExpected := "SYSTEM"
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUserInput, "the custom user should be different from the default user")
	s.installPreviousAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUserInput)),
	)
	// sanity check that the agent is running as the custom user
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUserInput)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedDomain", agentDomainExpected).
		WithValueEqual("installedUser", agentUserExpected).
		HasAService("datadogagent").
		WithIdentity(identity)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err = s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedDomain", agentDomainExpected).
		WithValueEqual("installedUser", agentUserExpected).
		HasAService("datadogagent").
		WithIdentity(identity)
}

// TestDowngradeWithMissingInstallSource tests that a downgrade will succeed even if the original install source is missing
func (s *testAgentUpgradeSuite) TestDowngradeWithMissingInstallSource() {
	s.T().Skip("Skipping test due to removal of update install source custom action")
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()
	s.removeWindowsInstallerCache()

	// Act
	s.MustStartExperimentPreviousVersion()
	s.AssertSuccessfulAgentStartExperiment(s.StableAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
}

func (s *testAgentUpgradeSuite) setWatchdogTimeout(timeout int) {
	// Set HKEY_LOCAL_MACHINE\SOFTWARE\Datadog\Datadog Agent\WatchdogTimeout to timeout
	err := windowscommon.SetRegistryDWORDValue(s.Env().RemoteHost, `HKLM:\SOFTWARE\Datadog\Datadog Agent`, "WatchdogTimeout", timeout)
	s.Require().NoError(err)
}

// setInstallerBootstrapMode sets the InstallerBootstrapMode registry key.
// - "OCI" forces the OCI bootstrap path, fails if installer layer is missing
// - "MSI" forces the MSI fallback path
// - "" (empty) uses default behavior (try OCI, fallback to MSI)
func (s *testAgentUpgradeSuite) setInstallerBootstrapMode(mode string) {
	err := windowscommon.SetTypedRegistryValue(s.Env().RemoteHost,
		`HKLM:\SOFTWARE\Datadog\Datadog Agent`, "InstallerBootstrapMode", mode, "String")
	s.Require().NoError(err)
}

func (s *testAgentUpgradeSuite) setTerminatePolicy(terminatePolicy bool) {
	termValue := 0
	if terminatePolicy {
		termValue = 1
	}
	// Set HKEY_LOCAL_MACHINE\SOFTWARE\Datadog\Datadog Agent\TerminatePolicy to terminatePolicy
	err := windowscommon.SetRegistryDWORDValue(s.Env().RemoteHost, `HKLM:\SOFTWARE\Datadog\Datadog Agent`, "TerminatePolicy", termValue)
	s.Require().NoError(err)
}

func (s *testAgentUpgradeSuite) installPreviousAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
	}
	options = append(options, opts...)
	s.InstallWithDiagnostics(options...)

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) installCurrentAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.CurrentAgentVersion().Version()

	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-current-version.log"),
	}
	options = append(options, opts...)
	s.InstallWithDiagnostics(options...)

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) setAgentConfig() {
	s.setAgentConfigWithAltDir("C:\\ProgramData\\Datadog")
}

func (s *testAgentUpgradeSuite) setAgentConfigWithAltDir(path string) {
	s.Env().RemoteHost.MkdirAll(path)
	configPath := path + `\datadog.yaml`
	// Ensure the API key is set for telemetry
	apiKey := installer.GetAPIKey()
	s.Env().RemoteHost.WriteFile(configPath, []byte(`
api_key: `+apiKey+`
site: datadoghq.com
remote_updates: true
log_level: debug
`))
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentStopExperiment(version string) {
	// conditions are same as promote, except the stable version should be unchanged.
	// since version is an input we can reuse.
	s.AssertSuccessfulAgentPromoteExperiment(version)
}

func (s *testAgentUpgradeSuite) waitForInstallerVersion(version string) error {
	// usually waiting after MSI runs so we have to wait awhile
	// max wait is 30*30 -> 900 seconds (15 minutes)
	return s.waitForInstallerVersionWithBackoff(version,
		backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(30))
}

func (s *testAgentUpgradeSuite) waitForInstallerVersionWithBackoff(version string, opts ...backoff.RetryOption) error {
	_, err := backoff.Retry(context.Background(), func() (any, error) {
		actual, err := s.Installer().Version()
		if err != nil {
			return nil, err
		}
		if !strings.Contains(actual, version) {
			return nil, fmt.Errorf("expected version %s, got %s", version, actual)
		}
		return nil, nil
	}, opts...)
	return err
}

// assertDaemonStaysRunning asserts that the daemon service PID and start time are the same before and after the function is called.
//
// For example, used to verify that "stop-experiment" does not reinstall stable when it is already installed.
func (s *testAgentUpgradeSuite) assertDaemonStaysRunning(f func()) {
	s.T().Helper()

	// service must be running before we can get the PID
	// might be redundant in some cases but we keep forgetting to ensure it
	// in others and it keeps causing flakes.
	s.Require().NoError(s.WaitForInstallerService("Running"))

	originalPID, err := windowscommon.GetServicePID(s.Env().RemoteHost, consts.ServiceName)
	s.Require().NoError(err)
	s.Require().Greater(originalPID, 0)

	originalStartTime, err := windowscommon.GetProcessStartTimeAsFileTimeUtc(s.Env().RemoteHost, originalPID)
	s.Require().NoError(err)

	f()

	newPID, err := windowscommon.GetServicePID(s.Env().RemoteHost, consts.ServiceName)
	s.Require().NoError(err)
	s.Require().Equal(originalPID, newPID, "daemon should not have been restarted (PID changed)")

	newStartTime, err := windowscommon.GetProcessStartTimeAsFileTimeUtc(s.Env().RemoteHost, newPID)
	s.Require().NoError(err)
	s.Require().Equal(originalStartTime, newStartTime, "daemon should not have been restarted (start time changed, PID reused)")
}

type testAgentUpgradeFromGASuite struct {
	testAgentUpgradeSuite
}

// TestAgentUpgradesFromGA tests that we can upgrade from GA release (7.65.0) to current
//
// NOTE: This test exercises the MSI fallback bootstrap path because the 7.65.x installer
// does not support extracting from the OCI installer layer - it only has the MSI admin install
// extraction flow. The dedicated test for validating the MSI fallback path with the current
// installer is TestUpgradeAgentPackageMSIBootstrap.
//
// It embeds testAgentUpgradeSuite so it can run any of the upgrade tests.
func TestAgentUpgradesFromGA(t *testing.T) {
	s := &testAgentUpgradeFromGASuite{}
	s.testAgentUpgradeSuite.BaseSuite.CreateStableAgent = s.createStableAgent
	e2e.Run(t, s,
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// BeforeTest wraps the installer in the DatadogInstallerGA type to handle the special cases for 7.65.x
func (s *testAgentUpgradeFromGASuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// set terminate policy to false to prevent the service from being forcefully terminated
	// this is to alert us to issues with agent termination that might be hidden by the default policy
	s.setTerminatePolicy(false)

	// Wrap the installer in the InstallerGA type to handle the special cases for 7.65.x
	s.SetInstaller(&installerwindows.DatadogInstallerGA{
		DatadogInstaller: s.Installer().(*installerwindows.DatadogInstaller),
	})
}

// createStableAgent provides AgentVersionManager for the 7.65.0 Agent release to the suite
func (s *testAgentUpgradeFromGASuite) createStableAgent() (*installerwindows.AgentVersionManager, error) {
	previousVersion := "7.65.2"
	previousVersionPackage := "7.65.2-1"

	// Get previous version OCI package
	previousOCI, err := installerwindows.NewPackageConfig(
		installerwindows.WithName(consts.AgentPackage),
		installerwindows.WithVersion(previousVersion),
		installerwindows.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup OCI package for previous agent version")

	// Get previous version MSI package
	url, err := windowsagent.GetChannelURL("stable")
	s.Require().NoError(err)
	previousMSI, err := windowsagent.NewPackage(
		windowsagent.WithVersion(previousVersionPackage),
		windowsagent.WithURLFromInstallersJSON(url, previousVersionPackage),
		windowsagent.WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup MSI for previous agent version")

	// Allow override of version and version package via environment variables
	// if not running in the CI, to reduce risk of accidentally using the wrong version in the CI.
	if os.Getenv("CI") == "" {
		if val := os.Getenv("STABLE_AGENT_VERSION"); val != "" {
			previousVersion = val
		}
		if val := os.Getenv("STABLE_AGENT_VERSION_PACKAGE"); val != "" {
			previousVersionPackage = val
		}
	}

	// Setup previous Agent artifacts
	agent, err := installerwindows.NewAgentVersionManager(
		previousVersion,
		previousVersionPackage,
		previousOCI,
		previousMSI,
	)
	s.Require().NoError(err, "Stable agent version was in an incorrect format")

	return agent, nil
}

// setExperimentMSIArgs stores a list of MSI options for the installer to provide to the MSI when starting an experiment.
func (s *testAgentUpgradeSuite) setExperimentMSIArgs(args []string) {
	err := windowscommon.SetRegistryMultiString(s.Env().RemoteHost, `HKLM:SOFTWARE\Datadog\Datadog Agent`, "StartExperimentMSIArgs", args)
	s.Require().NoError(err)
}

// removeWindowsInstallerCache clears the Windows Installer cache and install sources
func (s *testAgentUpgradeSuite) removeWindowsInstallerCache() {
	_, err := s.Env().RemoteHost.Execute("Remove-Item -Path C:\\Windows\\Installer\\* -Recurse -Force")
	s.Require().NoError(err)
	s.T().Logf("Removed Windows Installer cache")

	// Remove the MSI URL Install Source
	compressedProductCode, err := s.Env().RemoteHost.Execute("(@(Get-ChildItem -Path 'HKLM:SOFTWARE\\Classes\\Installer\\Products' -Recurse) | Where {$_.GetValue('ProductName') -like 'Datadog Agent' }).PSChildName")
	s.Require().NoError(err)
	s.T().Logf("Compressed Product Code: %s", compressedProductCode)

	registryKey := "Registry::HKEY_CLASSES_ROOT\\Installer\\Products\\" + compressedProductCode + "\\SourceList\\URL"
	s.T().Logf("Registry Key: %s", registryKey)
	exists, err := windowscommon.RegistryKeyExists(s.Env().RemoteHost, registryKey)
	s.Require().NoError(err)
	if exists {
		{
			err = windowscommon.DeleteRegistryKey(s.Env().RemoteHost, registryKey)
			s.Require().NoError(err)
			s.T().Logf("Removed MSI URL Install Source")
		}

	}

	registryKey = "Registry::HKEY_CLASSES_ROOT\\Installer\\Products\\" + compressedProductCode + "\\SourceList\\Net"
	err = windowscommon.SetTypedRegistryValue(s.Env().RemoteHost, registryKey, "2", "C:\\Windows\\FakePath", "ExpandString")
	s.Require().NoError(err)
	s.T().Logf("Set Fake MSI Install Source")

}
