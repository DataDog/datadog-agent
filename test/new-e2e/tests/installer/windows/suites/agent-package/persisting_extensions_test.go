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

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// stableExtensionsPipelineID is the pipeline used as the "stable" base for extension persistence tests.
// It must point to a build that already includes extension support (prerm/postinst hooks in the MSI).
// TODO: replace with a staging/GA release once extension support ships.
const stableExtensionsPipelineID = "98087596"

type testExtensionsSuite struct {
	installerwindows.BaseSuite
}

// TestExtensionPersistence tests Agent extension persistence behaviour on Windows.
func TestExtensionPersistence(t *testing.T) {
	s := &testExtensionsSuite{}
	s.CreateStableAgent = func() (*installerwindows.AgentVersionManager, error) {
		oci, err := installerwindows.NewPackageConfig(
			installerwindows.WithName(consts.AgentPackage),
			installerwindows.WithPipeline(stableExtensionsPipelineID),
		)
		if err != nil {
			return nil, err
		}
		msi, err := windowsagent.NewPackage(
			windowsagent.WithURLFromPipeline(stableExtensionsPipelineID),
		)
		if err != nil {
			return nil, err
		}
		// Version comes from the milestone at the time the pipeline was created.
		// The exact git suffix is irrelevant for the Contains() check used in installPreviousAgentVersion.
		return installerwindows.NewAgentVersionManager("7.77.0-devel", "7.77.0-devel.pipeline."+stableExtensionsPipelineID+"-1", oci, msi)
	}
	e2e.Run(t, s,
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
func (s *testExtensionsSuite) installExtension(pkg installerwindows.TestPackageConfig, extensionName string) {
	output, err := s.runInstallerCommand(fmt.Sprintf("extension install %s %s", pkg.URL(), extensionName))
	s.Require().NoError(err, "Failed to install extension: %s", output)
}

// removeExtension removes an extension using the datadog-installer.
func (s *testExtensionsSuite) removeExtension(packageName, extensionName string) {
	output, err := s.runInstallerCommand(fmt.Sprintf("extension remove %s %s", packageName, extensionName))
	s.Require().NoError(err, "Failed to remove extension: %s", output)
}

// verifyDDOTRunning verifies that DDOT is running via PowerShell service check.
// If expectedVersion is non-empty, also verifies the DDOT service binary path
// contains that version string, confirming the correct version is running.
func (s *testExtensionsSuite) verifyDDOTRunning(expectedVersion string) {
	assert.Eventually(s.T(), func() bool {
		output, err := s.Env().RemoteHost.Execute(
			`$svc = Get-Service -Name "datadog-otel-agent" -ErrorAction SilentlyContinue; if ($null -eq $svc) { Write-Output "NotFound" } else { Write-Output $svc.Status }`)
		if err != nil {
			return false
		}
		return strings.Contains(output, "Running")
	}, 60*time.Second, 2*time.Second, "DDOT service should be running")

	if expectedVersion != "" {
		binaryPath, err := s.Env().RemoteHost.Execute(
			`(Get-WmiObject -Class Win32_Service -Filter "Name='datadog-otel-agent'").PathName`)
		s.Require().NoError(err, "failed to get DDOT service binary path")
		s.Require().Contains(binaryPath, expectedVersion,
			"DDOT binary path should contain version %s", expectedVersion)
	}
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
installer:
  registry:
    url: installtesting.datad0g.com.internal.dda-testing.com
`))
}

// installPreviousAgentVersion installs the previous stable agent version.
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
	s.installPreviousAgentVersion()
	s.installExtension(s.StableAgentVersion().OCIPackage(), "ddot")
	defer func() {
		s.removeExtension("datadog-agent", "ddot")
	}()
	s.verifyDDOTRunning(s.StableAgentVersion().Version())
	s.installCurrentAgentVersion()
	s.verifyDDOTRunning(s.CurrentAgentVersion().Version())
}

// TestExtensionRestoredOnMSIRollback tests that extensions are restored when an MSI upgrade fails and rolls back.
//
// Scenario: Install previous MSI -> install extension -> upgrade with WIXFAILWHENDEFERRED=1
// -> verify rollback restores old version -> verify extension is restored
func (s *testExtensionsSuite) TestExtensionRestoredOnMSIRollback() {
	s.setAgentConfig()
	s.installPreviousAgentVersion()
	s.installExtension(s.StableAgentVersion().OCIPackage(), "ddot")
	defer func() {
		s.removeExtension("datadog-agent", "ddot")
	}()
	s.verifyDDOTRunning(s.StableAgentVersion().Version())

	err := windowscommon.SetRegistryMultiString(s.Env().RemoteHost,
		`HKLM:SOFTWARE\Datadog\Datadog Agent`, "StartExperimentMSIArgs",
		[]string{"WIXFAILWHENDEFERRED=1"})
	s.Require().NoError(err)

	s.WaitForDaemonToStop(func() {
		_, err := s.StartExperimentCurrentVersion()
		s.Require().NoError(err, "daemon should stop cleanly")
	})

	err = s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	s.verifyDDOTRunning(s.StableAgentVersion().Version())
}

// TestExtensionRemovedOnUninstall tests that extensions are cleaned up on uninstall.
//
// Scenario: Install MSI -> install extension -> uninstall MSI -> verify extension removed
func (s *testExtensionsSuite) TestExtensionRemovedOnUninstall() {
	s.setAgentConfig()

	// 1. Install current agent version
	s.installCurrentAgentVersion()

	// 2. Install DDOT extension
	s.installExtension(s.CurrentAgentVersion().OCIPackage(), "ddot")

	// 3. Verify DDOT is running
	s.verifyDDOTRunning(s.CurrentAgentVersion().Version())

	// 4. Uninstall agent
	err := s.Installer().Uninstall()
	s.Require().NoError(err, "Failed to uninstall agent")

	// 5. Verify DDOT service is removed
	s.verifyDDOTServiceNotRunning()
}
