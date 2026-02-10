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

// TestDDOTExtension tests installing DDOT as an extension on all platforms
func (s *extensionsSuite) TestDDOTExtension() {
	// Install base agent
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	output, err := s.Installer.InstallExtension(s.getAgentPackageURL(), "ddot")
	s.Require().NoError(err, "Failed to install DDOT extension: %s", output)
	defer func() {
		_, _ = s.Installer.RemoveExtension("datadog-agent", "ddot")
	}()

	// Verify extension directory exists
	extensionPath := s.getExtensionPath("datadog-agent", "stable", "ddot")
	exists, err := s.Host.DirExists(extensionPath)
	s.Require().NoError(err)
	s.Require().True(exists, "DDOT extension directory should exist at %s", extensionPath)

	// Verify binary exists (platform-specific path)
	binaryPath := s.getDDOTBinaryPath(extensionPath)
	exists, err = s.Env().RemoteHost.FileExists(binaryPath)
	s.Require().NoError(err)
	s.Require().True(exists, "DDOT binary should exist at %s", binaryPath)

	// Platform-specific verification
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		s.verifyDDOTExtensionLinux(extensionPath)
	case e2eos.WindowsFamily:
		s.verifyDDOTExtensionWindows()
	}

	// Remove extension
	output, err = s.Installer.RemoveExtension("datadog-agent", "ddot")
	s.Require().NoError(err, "Failed to remove DDOT extension: %s", output)

	// Verify cleanup
	exists, err = s.Host.DirExists(extensionPath)
	s.Require().NoError(err)
	s.Require().False(exists, "DDOT extension should be removed")

	// Platform-specific cleanup verification
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.WindowsFamily:
		s.verifyDDOTServiceRemoved()
	}
}

// Helper methods

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
func (s *extensionsSuite) getAgentPackageURL() string {
	// Use pipeline-specific URL for E2E tests
	pipelineID := os.Getenv("E2E_PIPELINE_ID")
	if pipelineID == "" {
		s.T().Fatal("E2E_PIPELINE_ID environment variable not set")
	}
	return "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-" + pipelineID
}

// getDDOTBinaryPath returns the platform-specific DDOT binary path
func (s *extensionsSuite) getDDOTBinaryPath(extensionPath string) string {
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		return filepath.Join(extensionPath, "embedded", "bin", "otel-agent")
	case e2eos.WindowsFamily:
		return filepath.Join(extensionPath, "embedded", "bin", "otel-agent.exe")
	default:
		s.T().Fatalf("unsupported OS: %v", s.Env().RemoteHost.OSFamily)
		return ""
	}
}

// verifyDDOTExtensionLinux verifies DDOT extension on Linux
func (s *extensionsSuite) verifyDDOTExtensionLinux(extensionPath string) {
	// Verify otel-config.yaml.example exists
	examplePath := "/etc/datadog-agent/otel-config.yaml.example"
	exists, err := s.Env().RemoteHost.FileExists(examplePath)
	s.Require().NoError(err)
	s.Require().True(exists, "otel-config.yaml.example should exist")

	// Verify otel-config.yaml created
	configPath := "/etc/datadog-agent/otel-config.yaml"
	exists, err = s.Env().RemoteHost.FileExists(configPath)
	s.Require().NoError(err)
	s.Require().True(exists, "otel-config.yaml should exist")

	// Verify otel-config.yaml permissions
	output, err := s.Env().RemoteHost.Execute("stat -c '%a %U %G' " + configPath)
	s.Require().NoError(err)
	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) != 3 {
		s.Require().Fail("unexpected stat output: %s", output)
	}
	s.Require().Equal("640", parts[0], "otel-config.yaml should be restricted (640)")
	s.Require().Equal("dd-agent", output[1], "otel-config.yaml should be owned by dd-agent")
	s.Require().Equal("dd-agent", output[2], "otel-config.yaml should have group dd-agent")

	// Verify extension ownership (tests postInstallExtension hook)
	output, err = s.Env().RemoteHost.Execute("stat -c '%U:%G' " + extensionPath)
	s.Require().NoError(err)
	s.Require().Contains(output, "dd-agent:dd-agent", "Extension should be owned by dd-agent")

	// Verify DDOT process is running (check for otel-agent process)
	assert.Eventually(s.T(), func() bool {
		output, err := s.Env().RemoteHost.Execute("sudo systemctl is-active --quiet datadog-agent-ddot && echo running || true")
		return err == nil && strings.Contains(output, "running")
	}, 30*time.Second, 1*time.Second, "DDOT (otel-agent) process should be running")

	// Verify datadog.yaml has otelcollector enabled
	ddYamlPath := "/etc/datadog-agent/datadog.yaml"
	content, err := s.Env().RemoteHost.ReadFilePrivileged(ddYamlPath)
	s.Require().NoError(err)
	s.Require().Contains(content, "otelcollector:\n  enabled: true",
		"datadog.yaml should have otelcollector enabled")
}

// verifyDDOTExtensionWindows verifies DDOT extension on Windows
func (s *extensionsSuite) verifyDDOTExtensionWindows() {
	// Verify otel-config.yaml created
	configPath := `C:\ProgramData\Datadog\otel-config.yaml`
	exists, err := s.Env().RemoteHost.FileExists(configPath)
	s.Require().NoError(err)
	s.Require().True(exists, "otel-config.yaml should exist")

	// Verify datadog.yaml has otelcollector enabled
	ddYamlPath := `C:\ProgramData\Datadog\datadog.yaml`
	content, err := s.Env().RemoteHost.ReadFilePrivileged(ddYamlPath)
	s.Require().NoError(err)
	// Check for otelcollector section with enabled: true (account for Windows line endings)
	contentStr := string(content)
	s.Require().True(
		strings.Contains(contentStr, "otelcollector:\n  enabled: true") ||
			strings.Contains(contentStr, "otelcollector:\r\n  enabled: true"),
		"datadog.yaml should have otelcollector enabled")

	// Verify ddot service is running
	assert.Eventually(s.T(), func() bool {
		output, err := s.Env().RemoteHost.Execute("Get-Service -Name datadog-otel-agent | Select-Object -ExpandProperty Status")
		return err == nil && strings.TrimSpace(output) == "Running"
	}, 30*time.Second, 1*time.Second, "DDOT service should be running")
}

// verifyDDOTServiceRemoved verifies DDOT service removal on Windows
func (s *extensionsSuite) verifyDDOTServiceRemoved() {
	// Wait for service to be removed
	assert.Eventually(s.T(), func() bool {
		output, err := s.Env().RemoteHost.Execute(`$svc = Get-Service -Name "datadog-otel-agent" -ErrorAction SilentlyContinue; if ($null -eq $svc) { Write-Output "NotFound" } else { Write-Output $svc.Status }`)
		return err == nil && strings.Contains(output, "NotFound")
	}, 30*time.Second, 1*time.Second, "DDOT service should be removed")
}
