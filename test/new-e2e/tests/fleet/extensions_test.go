// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

type extensionsSuite struct {
	suite.FleetSuite
	fixtureServer *fixtures.Server
	packageURL    string // Remote package URL with extension fixture
}

func newExtensionsSuite() e2e.Suite[environments.Host] {
	return &extensionsSuite{}
}

func TestFleetExtensions(t *testing.T) {
	suite.Run(t, newExtensionsSuite, suite.AllPlatforms)
}

func (s *extensionsSuite) SetupSuite() {
	s.FleetSuite.SetupSuite()
	s.fixtureServer = fixtures.NewServer(s.T())

	// Copy fixture to remote VM once for all tests
	localLayoutPath := s.fixtureServer.PackageLayoutURL(fixtures.FixtureSimpleV1WithExtension)
	localLayoutPath = strings.TrimPrefix(localLayoutPath, "file://")

	remoteLayoutPath := "/tmp/oci-layout-simple-v1-with-extension"
	err := s.Env().RemoteHost.CopyFolder(localLayoutPath, remoteLayoutPath)
	s.Require().NoError(err, "Failed to copy fixture to VM")

	s.packageURL = "file://" + remoteLayoutPath
}

// TestExtensionInstallAndRemove tests installing and removing an extension
func (s *extensionsSuite) TestExtensionInstallAndRemove() {
	// Install agent with datadog-installer
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Install package with extension directly from file:// URL (without using catalog)
	output, err := s.Installer.Install(s.packageURL)
	s.Require().NoError(err, "Failed to install package: %s", output)
	defer func() {
		// Cleanup: remove package
		_, _ = s.Installer.Remove("simple")
	}()

	// Install extension
	output, err = s.Installer.InstallExtension(s.packageURL, "simple-extension")
	s.Require().NoError(err, "Failed to install extension: %s", output)

	// Verify extension was installed
	extensionPath := s.getExtensionPath("simple", "v1", "simple-extension")
	exists, err := s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists")
	s.Require().True(exists, "Extension should be installed at %s", extensionPath)

	// Verify extension script file exists
	scriptPath := filepath.Join(extensionPath, "extension.sh")
	exists, err = s.Env().RemoteHost.FileExists(scriptPath)
	s.Require().NoError(err, "Failed to check if extension script exists")
	s.Require().True(exists, "Extension script should exist at %s", scriptPath)

	// Remove extension
	output, err = s.Installer.RemoveExtension("simple", "simple-extension")
	s.Require().NoError(err, "Failed to remove extension: %s", output)

	// Verify extension was removed
	exists, err = s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists")
	s.Require().False(exists, "Extension should be removed from %s", extensionPath)
}

// TestExtensionSaveAndRestore tests saving and restoring extensions
func (s *extensionsSuite) TestExtensionSaveAndRestore() {
	// Install agent with datadog-installer
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Install package with extension directly from file:// URL (without using catalog)
	output, err := s.Installer.Install(s.packageURL)
	s.Require().NoError(err, "Failed to install package: %s", output)
	defer func() {
		// Cleanup: remove package
		_, _ = s.Installer.Remove("simple")
	}()

	// Install extension
	output, err = s.Installer.InstallExtension(s.packageURL, "simple-extension")
	s.Require().NoError(err, "Failed to install extension: %s", output)

	// Verify extension was installed
	extensionPath := s.getExtensionPath("simple", "v1", "simple-extension")
	exists, err := s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists")
	s.Require().True(exists, "Extension should be installed at %s", extensionPath)

	// Create save directory in temp folder
	tmpFolder, err := s.Env().RemoteHost.GetTmpFolder()
	s.Require().NoError(err, "Failed to get temp folder")
	saveDir := s.Env().RemoteHost.JoinPath(tmpFolder, "extensions-save-test")
	err = s.Env().RemoteHost.MkdirAll(saveDir)
	s.Require().NoError(err, "Failed to create save directory")

	// Save extensions to temp directory
	output, err = s.Installer.SaveExtensions("simple", saveDir)
	s.Require().NoError(err, "Failed to save extensions: %s", output)
	defer func() {
		// Cleanup: remove save directory
		_ = s.Env().RemoteHost.RemoveAll(saveDir)
	}()

	// Verify save directory was created
	saveExists, err := s.Host.DirExists(saveDir)
	s.Require().NoError(err, "Failed to check if save directory exists")
	s.Require().True(saveExists, "Save directory should be created at %s", saveDir)

	// Remove extension
	output, err = s.Installer.RemoveExtension("simple", "simple-extension")
	s.Require().NoError(err, "Failed to remove extension: %s", output)

	// Verify extension was removed
	exists, err = s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists after removal")
	s.Require().False(exists, "Extension should be removed from %s", extensionPath)

	// Restore extensions from save directory
	output, err = s.Installer.RestoreExtensions(s.packageURL, saveDir)
	s.Require().NoError(err, "Failed to restore extensions: %s", output)

	// Verify extension was restored
	exists, err = s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists after restore")
	s.Require().True(exists, "Extension should be restored at %s", extensionPath)
}

// TestExtensionSurvivesExperiment verifies that extensions installed on the
// datadog-agent package survive an upgrade via the experiment (start/promote) flow.
func (s *extensionsSuite) TestExtensionSurvivesExperiment() {
	s.Agent.MustInstall(agent.WithPipelineID("98077405")) // TODO: use staging package for persistence
	defer s.Agent.MustUninstall()

	s.Installer.MustInstallExtension(s.getAgentPackageURL("98077405"), "ddot")
	defer func() {
		_, _ = s.Installer.RemoveExtension("datadog-agent", "ddot")
	}()
	s.verifyDDOTRunning()
	s.setInstallerRegistryConfig()

	targetVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err := s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	s.verifyDDOTRunning()

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.verifyDDOTRunning()
}

// TestExtensionRestoredAfterExperimentRollback verifies that extensions are
// restored to their stable state when an experiment is stopped (rolled back).
func (s *extensionsSuite) TestExtensionRestoredAfterExperimentRollback() {
	s.Agent.MustInstall(agent.WithPipelineID("98077405")) // TODO: use staging package for persistence
	defer s.Agent.MustUninstall()

	s.Installer.MustInstallExtension(s.getAgentPackageURL("98077405"), "ddot")
	defer func() {
		_, _ = s.Installer.RemoveExtension("datadog-agent", "ddot")
	}()

	s.verifyDDOTRunning()
	s.setInstallerRegistryConfig()

	targetVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err := s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	s.verifyDDOTRunning()

	err = s.Backend.StopExperiment("datadog-agent")
	s.Require().NoError(err)
	s.verifyDDOTRunning()
}

// TestDDOTExtension tests installing DDOT as an extension on all platforms
func (s *extensionsSuite) TestDDOTExtension() {
	// Install base agent
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	s.Installer.MustInstallExtension(s.getAgentPackageURL(""), "ddot")
	defer func() {
		_, _ = s.Installer.RemoveExtension("datadog-agent", "ddot")
	}()

	// Verify DDOT is running via status
	s.verifyDDOTRunning()

	// Remove extension
	s.Installer.MustRemoveExtension("datadog-agent", "ddot")

	// Platform-specific cleanup verification
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.WindowsFamily:
		s.verifyDDOTServiceRemoved()
	}
}

// Helper methods

// setInstallerRegistryConfig appends the installer registry URL to datadog.yaml.
// It is idempotent: it will not append the URL if it is already present.
func (s *extensionsSuite) setInstallerRegistryConfig() {
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		_, err := s.Env().RemoteHost.Execute(`grep -q "installtesting.datad0g.com.internal.dda-testing.com" /etc/datadog-agent/datadog.yaml || sudo sh -c 'printf "\ninstaller:\n  registry:\n    url: installtesting.datad0g.com.internal.dda-testing.com\n" >> /etc/datadog-agent/datadog.yaml'`)
		s.Require().NoError(err)
	case e2eos.WindowsFamily:
		_, err := s.Env().RemoteHost.Execute("if (-not (Select-String -Path \"C:\\ProgramData\\Datadog\\datadog.yaml\" -Pattern \"installtesting.datad0g.com.internal.dda-testing.com\" -Quiet)) { Add-Content \"C:\\ProgramData\\Datadog\\datadog.yaml\" -Value (\"`ninstaller:`n  registry:`n    url: installtesting.datad0g.com.internal.dda-testing.com\") }")
		s.Require().NoError(err)
	}
}

// getExtensionPath returns the path to an extension directory.
// It uses the same logic as pkg/fleet/installer/packages/extensions/extensions.go:getExtensionsPath
func (s *extensionsSuite) getExtensionPath(pkg, version, extensionName string) string {
	var basePath string
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		basePath = filepath.Join(paths.PackagesPath, pkg, version)
	case e2eos.WindowsFamily:
		// On Windows: C:\ProgramData\Datadog\Installer\packages\<pkg>\<version>
		basePath = filepath.Join(`C:\ProgramData\Datadog\Installer\packages`, pkg, version)
	default:
		s.T().Fatalf("unsupported OS family: %v", s.Env().RemoteHost.OSFamily)
		return ""
	}
	return filepath.Join(basePath, "ext", extensionName)
}

// getAgentPackageURL returns the platform-specific agent package URL
func (s *extensionsSuite) getAgentPackageURL(version string) string {
	if version == "" {
		// Use pipeline-specific URL for E2E tests
		version = os.Getenv("E2E_PIPELINE_ID")
		if version == "" {
			s.T().Fatal("E2E_PIPELINE_ID environment variable not set")
		}
	}
	return "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-" + version
}

// verifyDDOTRunning verifies DDOT is running via agent status
func (s *extensionsSuite) verifyDDOTRunning() {
	isDDOTRunning := assert.Eventually(s.T(), func() bool {
		status, err := s.Agent.Status()
		if err != nil {
			return false
		}

		// Check that DDOT is not in error state
		if status.OtelAgent.Error != "" {
			s.T().Logf("DDOT error: %s", status.OtelAgent.Error)
			return false
		}

		// Verify required fields are present
		if status.OtelAgent.AgentVersion == "" || status.OtelAgent.CollectorVersion == "" {
			s.T().Logf("Missing DDOT version info")
			return false
		}

		return true
	}, 30*time.Second, 1*time.Second, "DDOT should be running and reporting status")
	if !isDDOTRunning {
		s.T().Fatalf("DDOT is not running")
	}

	// Log version info for debugging
	status, _ := s.Agent.Status()
	s.T().Logf("DDOT AgentVersion: %s, CollectorVersion: %s",
		status.OtelAgent.AgentVersion, status.OtelAgent.CollectorVersion)
}

// verifyDDOTServiceRemoved verifies DDOT service removal on Windows
func (s *extensionsSuite) verifyDDOTServiceRemoved() {
	// Wait for service to be removed
	isDDOTRemoved := assert.Eventually(s.T(), func() bool {
		output, err := s.Env().RemoteHost.Execute(`$svc = Get-Service -Name "datadog-otel-agent" -ErrorAction SilentlyContinue; if ($null -eq $svc) { Write-Output "NotFound" } else { Write-Output $svc.Status }`)
		return err == nil && strings.Contains(output, "NotFound")
	}, 30*time.Second, 1*time.Second, "DDOT service should be removed")
	if !isDDOTRemoved {
		s.T().Fatalf("DDOT service should be removed")
	}
}
