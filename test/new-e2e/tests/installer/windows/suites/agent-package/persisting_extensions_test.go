// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

type testExtensionsSuite struct {
	installerwindows.BaseSuite
}

// TestExtensionPersistence tests that Agent extensions persist through MSI upgrade,
// experiment, uninstall, and rollback scenarios.
func TestExtensionPersistence(t *testing.T) {
	e2e.Run(t, &testExtensionsSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// runInstallerCommand runs a datadog-installer.exe command on the remote host.
func (s *testExtensionsSuite) runInstallerCommand(args string) (string, error) {
	binaryPath := fmt.Sprintf(`%s\%s`, consts.Path, consts.BinaryName)
	envVars := fmt.Sprintf(`$env:DD_API_KEY='%s'; $env:DD_SITE='datadoghq.com'; `, installer.GetAPIKey())
	cmd := fmt.Sprintf(`%s& "%s" %s`, envVars, binaryPath, args)
	return s.Env().RemoteHost.Execute(cmd)
}

// installExtension installs an extension using the datadog-installer.
func (s *testExtensionsSuite) installExtension(packageURL, extensionName string) {
	output, err := s.runInstallerCommand(fmt.Sprintf("extension install %s %s", packageURL, extensionName))
	s.Require().NoError(err, "Failed to install extension: %s", output)
}

// removeExtension removes an extension.
func (s *testExtensionsSuite) removeExtension(packageName, extensionName string) {
	output, err := s.runInstallerCommand(fmt.Sprintf("extension remove %s %s", packageName, extensionName))
	s.Require().NoError(err, "Failed to remove extension: %s", output)
}

// getAgentPackageURL returns the OCI URL for the current agent version.
func (s *testExtensionsSuite) getAgentPackageURL() string {
	return s.CurrentAgentVersion().OCIPackage().URL()
}

// verifyDDOTRunning verifies that DDOT is running via PowerShell service check.
func (s *testExtensionsSuite) verifyDDOTRunning() {
	assert.Eventually(s.T(), func() bool {
		output, err := s.Env().RemoteHost.Execute(
			`$svc = Get-Service -Name "datadog-otel-agent" -ErrorAction SilentlyContinue; if ($null -eq $svc) { Write-Output "NotFound" } else { Write-Output $svc.Status }`)
		if err != nil {
			return false
		}
		return strings.Contains(output, "Running")
	}, 60*time.Second, 2*time.Second, "DDOT service should be running")
}

// verifyDDOTServiceNotRunning verifies that the DDOT service is not present.
func (s *testExtensionsSuite) verifyDDOTServiceNotRunning() {
	output, err := s.Env().RemoteHost.Execute(
		`$svc = Get-Service -Name "datadog-otel-agent" -ErrorAction SilentlyContinue; if ($null -eq $svc) { Write-Output "NotFound" } else { Write-Output $svc.Status }`)
	s.Require().NoError(err)
	s.Require().Contains(output, "NotFound", "DDOT service should not exist")
}

// setAgentConfig creates the agent configuration.
func (s *testExtensionsSuite) setAgentConfig() {
	configPath := `C:\ProgramData\Datadog\datadog.yaml`
	s.Env().RemoteHost.MkdirAll(`C:\ProgramData\Datadog`)
	apiKey := installer.GetAPIKey()
	s.Env().RemoteHost.WriteFile(configPath, []byte(`
api_key: `+apiKey+`
site: datadoghq.com
remote_updates: true
log_level: debug
`))
}

// installPreviousAgentVersion installs the previous (stable) agent version.
func (s *testExtensionsSuite) installPreviousAgentVersion(opts ...installerwindows.MsiOption) {
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
	}
	options = append(options, opts...)
	s.InstallWithDiagnostics(options...)

	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
}

// installCurrentAgentVersion installs the current agent version.
func (s *testExtensionsSuite) installCurrentAgentVersion(opts ...installerwindows.MsiOption) {
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-current-version.log"),
	}
	options = append(options, opts...)
	s.InstallWithDiagnostics(options...)

	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

// TestExtensionPersistThroughMSIUpgrade tests that extensions survive a manual MSI upgrade.
//
// Scenario: Install previous MSI -> install extension -> upgrade MSI -> verify extension restored
func (s *testExtensionsSuite) TestExtensionPersistThroughMSIUpgrade() {
	s.setAgentConfig()

	// 1. Install previous agent version
	s.installPreviousAgentVersion()

	// 2. Install DDOT extension
	s.installExtension(s.getAgentPackageURL(), "ddot")
	defer func() {
		s.removeExtension("datadog-agent", "ddot")
	}()

	// 3. Verify DDOT is running
	s.verifyDDOTRunning()

	// 4. Upgrade to current agent version
	s.installCurrentAgentVersion()

	// 5. Verify DDOT extension survived the upgrade
	s.verifyDDOTRunning()
}

// TestExtensionPersistThroughExperiment tests that extensions survive an experiment
// (start/promote) flow.
//
// Scenario: Install previous MSI -> install extension -> start experiment -> promote -> verify extension
func (s *testExtensionsSuite) TestExtensionPersistThroughExperiment() {
	s.setAgentConfig()

	// 1. Install previous agent version
	s.installPreviousAgentVersion()

	// 2. Install DDOT extension
	s.installExtension(s.getAgentPackageURL(), "ddot")
	defer func() {
		s.removeExtension("datadog-agent", "ddot")
	}()

	// 3. Verify DDOT is running
	s.verifyDDOTRunning()

	// 4. Upgrade via experiment start + promote
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// 5. Verify DDOT extension survived the upgrade
	s.verifyDDOTRunning()
}

// TestExtensionRemovedOnUninstall tests that extensions are cleaned up on uninstall.
//
// Scenario: Install MSI -> install extension -> uninstall MSI -> verify extension removed
func (s *testExtensionsSuite) TestExtensionRemovedOnUninstall() {
	s.setAgentConfig()

	// 1. Install current agent version
	s.installCurrentAgentVersion()

	// 2. Install DDOT extension
	s.installExtension(s.getAgentPackageURL(), "ddot")

	// 3. Verify DDOT is running
	s.verifyDDOTRunning()

	// 4. Uninstall agent
	err := s.Installer().Uninstall()
	s.Require().NoError(err, "Failed to uninstall agent")

	// 5. Verify DDOT service is removed
	s.verifyDDOTServiceNotRunning()
}

// TestExtensionRestoredOnMSIRollback tests that extensions are restored when an MSI
// upgrade fails and rolls back.
//
// Scenario: Install previous MSI -> install extension -> upgrade with WIXFAILWHENDEFERRED=1
//
//	-> verify rollback restores old version -> verify extension is restored
func (s *testExtensionsSuite) TestExtensionRestoredOnMSIRollback() {
	s.setAgentConfig()

	// 1. Install previous agent version
	s.installPreviousAgentVersion()

	// 2. Install DDOT extension
	s.installExtension(s.getAgentPackageURL(), "ddot")
	defer func() {
		s.removeExtension("datadog-agent", "ddot")
	}()

	// 3. Verify DDOT is running before triggering rollback
	s.verifyDDOTRunning()

	// 4. Set experiment MSI args to trigger failure (WIXFAILWHENDEFERRED=1)
	err := windowscommon.SetRegistryMultiString(s.Env().RemoteHost,
		`HKLM:SOFTWARE\Datadog\Datadog Agent`, "StartExperimentMSIArgs",
		[]string{"WIXFAILWHENDEFERRED=1"})
	s.Require().NoError(err)

	// 5. Start experiment - this will fail during MSI install and trigger rollback
	s.WaitForDaemonToStop(func() {
		_, err := s.StartExperimentCurrentVersion()
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(5*time.Second)), backoff.WithMaxTries(100))

	// 6. Wait for the service to come back after rollback
	err = s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	// 7. Verify old version is still running
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	// 8. Verify DDOT extension is restored after rollback
	s.verifyDDOTRunning()
}
