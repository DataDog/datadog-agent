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

	// Get installed agent version
	agentVersion := "stable"

	// Install DDOT extension from the agent package
	// Note: In production, the agent OCI package includes DDOT as an extension layer
	// For E2E tests, we assume the package is already available
	packageURL := s.getAgentPackageURL()

	// Install DDOT extension from the package
	output, err := s.Installer.InstallExtension(packageURL, "ddot")
	if err != nil {
		// Log the installation error for debugging
		s.T().Logf("DDOT extension installation error: %v", err)
		s.T().Logf("Installation output: %s", output)
	}
	defer func() {
		_, _ = s.Installer.RemoveExtension("datadog-agent", "ddot")
	}()

	// Fail now if installation returned an error
	s.Require().NoError(err, "Failed to install DDOT extension")

	// Windows: Restart agent services to enable otelcollector endpoint (port 5009)
	// The DDOT service needs this endpoint for config sync
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		s.T().Log("Restarting agent services to enable otelcollector endpoint")
		_, err := s.Env().RemoteHost.Execute(`Restart-Service datadogagent -Force`)
		s.Require().NoError(err, "Failed to restart agent services")

		// Wait for agent to fully restart
		_, _ = s.Env().RemoteHost.Execute(`Start-Sleep -Seconds 5`)

		// Start the DDOT service now that the agent is listening on port 5009
		s.T().Log("Starting DDOT service")
		_, err = s.Env().RemoteHost.Execute(`Start-Service datadog-otel-agent`)
		s.Require().NoError(err, "Failed to start DDOT service")

		// Wait for DDOT service to fully start
		_, _ = s.Env().RemoteHost.Execute(`Start-Sleep -Seconds 3`)
	}

	// Run diagnostics after restart (useful for local debugging)
	extensionPath := s.getExtensionPath("datadog-agent", agentVersion, "ddot")
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		s.logWindowsDiagnostics(extensionPath)
	}

	// Verify extension directory exists
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
	// Verify otel-config.yaml created
	configPath := "/etc/datadog-agent/otel-config.yaml"
	exists, err := s.Env().RemoteHost.FileExists(configPath)
	s.Require().NoError(err)
	s.Require().True(exists, "otel-config.yaml should exist")

	// Verify extension ownership (tests postInstallExtension hook)
	output, err := s.Env().RemoteHost.Execute("stat -c '%U:%G' " + extensionPath)
	s.Require().NoError(err)
	s.Require().Contains(output, "dd-agent:dd-agent", "Extension should be owned by dd-agent")
}

// logWindowsDiagnostics logs diagnostic information for Windows debugging
func (s *extensionsSuite) logWindowsDiagnostics(extensionPath string) {
	s.T().Log("=== Windows Diagnostics ===")

	// Check main agent service status
	s.T().Log("--- Main Agent Service Status ---")
	output, err := s.Env().RemoteHost.Execute(`Get-Service -Name "datadogagent" -ErrorAction SilentlyContinue | Format-List`)
	if err != nil {
		s.T().Logf("Error querying main agent service: %v", err)
		s.T().Logf("Output: %s", output)
	} else if output == "" {
		s.T().Log("Main agent service not found")
	} else {
		s.T().Logf("Main agent service:\n%s", output)
	}

	// Check DDOT service status
	s.T().Log("--- DDOT Service Status ---")
	output, err = s.Env().RemoteHost.Execute(`sc query "datadog-otel-agent"`)
	if err != nil {
		s.T().Logf("Error querying DDOT service: %v", err)
		s.T().Logf("Output: %s", output)
	} else {
		s.T().Logf("DDOT service:\n%s", output)
	}

	// Also try Get-Service to get more details
	output2, err2 := s.Env().RemoteHost.Execute(`Get-Service -Name "datadog-otel-agent" -ErrorAction SilentlyContinue | Format-List`)
	if err2 != nil {
		s.T().Logf("Get-Service error: %v", err2)
	}
	if output2 != "" {
		s.T().Logf("Get-Service output:\n%s", output2)
	} else {
		s.T().Log("Get-Service returned empty (service may not exist)")
	}

	// Check service configuration (binary path, credentials)
	s.T().Log("--- DDOT Service Configuration ---")
	output3, err3 := s.Env().RemoteHost.Execute(`sc qc "datadog-otel-agent"`)
	if err3 != nil {
		s.T().Logf("Error querying service config: %v", err3)
	} else {
		s.T().Logf("Service config:\n%s", output3)
	}

	// Check if API key is configured
	s.T().Log("--- API Key Configuration ---")
	ddYamlPath := `C:\ProgramData\Datadog\datadog.yaml`
	content, err := s.Env().RemoteHost.ReadFile(ddYamlPath)
	if err != nil {
		s.T().Logf("Error reading datadog.yaml: %v", err)
	} else {
		// Check if api_key is set (don't log the actual key)
		if strings.Contains(string(content), "api_key:") {
			s.T().Log("API key is configured in datadog.yaml")
		} else {
			s.T().Log("WARNING: api_key not found in datadog.yaml")
		}
		// Check otelcollector config
		if strings.Contains(string(content), "otelcollector:") {
			s.T().Log("otelcollector section found in datadog.yaml")
			if strings.Contains(string(content), "enabled: true") {
				s.T().Log("otelcollector is enabled")
			}
		}
	}

	// Check extension directory contents
	s.T().Log("--- Extension Directory Contents ---")
	output, err = s.Env().RemoteHost.Execute(`Get-ChildItem -Path "` + extensionPath + `" -Recurse | Select-Object -ExpandProperty FullName`)
	if err != nil {
		s.T().Logf("Error listing extension directory: %v", err)
		s.T().Logf("Output: %s", output)
	} else {
		s.T().Logf("Extension files:\n%s", output)
	}

	// Check if otel-config.yaml exists
	s.T().Log("--- OTel Configuration ---")
	configPath := `C:\ProgramData\Datadog\otel-config.yaml`
	exists, err := s.Env().RemoteHost.FileExists(configPath)
	if err != nil {
		s.T().Logf("Error checking otel-config.yaml: %v", err)
	} else if exists {
		s.T().Log("otel-config.yaml exists")
	} else {
		s.T().Log("WARNING: otel-config.yaml not found")
	}

	// Check Windows Event Log for DDOT service errors
	s.T().Log("--- DDOT Service Event Log (last 10 errors) ---")
	output, err = s.Env().RemoteHost.Execute(`Get-EventLog -LogName System -Source "Service Control Manager" -EntryType Error -Newest 10 | Where-Object { $_.Message -like "*datadog-otel-agent*" } | Format-List TimeGenerated,Message`)
	if err != nil {
		s.T().Logf("Error reading event log: %v", err)
	} else if output == "" {
		s.T().Log("No recent DDOT service errors in event log")
	} else {
		s.T().Logf("DDOT service errors:\n%s", output)
	}

	// Try to manually start the service and see what happens
	s.T().Log("--- Attempting Manual Service Start ---")
	output, err = s.Env().RemoteHost.Execute(`Start-Service -Name "datadog-otel-agent" -ErrorAction SilentlyContinue; $?`)
	if err != nil {
		s.T().Logf("Error starting service: %v", err)
		s.T().Logf("Output: %s", output)
	} else {
		s.T().Logf("Start-Service result: %s", output)
		// Wait a moment and check status again
		output2, _ := s.Env().RemoteHost.Execute(`Start-Sleep -Seconds 2; Get-Service -Name "datadog-otel-agent" | Format-List Status`)
		s.T().Logf("Service status after manual start:\n%s", output2)
	}

	// Check installer logs for errors
	s.T().Log("--- Installer Logs (last 50 lines) ---")
	logPath := `C:\ProgramData\Datadog\Installer\installer.log`
	logExists, err := s.Env().RemoteHost.FileExists(logPath)
	if err == nil && logExists {
		output, err = s.Env().RemoteHost.Execute(`Get-Content "` + logPath + `" -Tail 50`)
		if err != nil {
			s.T().Logf("Error reading installer log: %v", err)
		} else {
			s.T().Logf("Installer log:\n%s", output)
		}
	} else {
		s.T().Log("Installer log not found")
	}

	s.T().Log("=== End Diagnostics ===")
}

// verifyDDOTExtensionWindows verifies DDOT extension on Windows
func (s *extensionsSuite) verifyDDOTExtensionWindows() {
	// Verify otel-config.yaml created
	configPath := `C:\ProgramData\Datadog\otel-config.yaml`
	exists, err := s.Env().RemoteHost.FileExists(configPath)
	s.Require().NoError(err)
	s.Require().True(exists, "otel-config.yaml should exist")

	// Verify Windows service created and running
	serviceName := "datadog-otel-agent"
	output, err := s.Env().RemoteHost.Execute(`sc query "` + serviceName + `"`)
	s.Require().NoError(err, "DDOT service should exist")
	s.Require().Contains(output, "RUNNING", "DDOT service should be running")

	// Verify datadog.yaml has otelcollector enabled
	ddYamlPath := `C:\ProgramData\Datadog\datadog.yaml`
	content, err := s.Env().RemoteHost.ReadFile(ddYamlPath)
	s.Require().NoError(err)
	s.Require().Contains(string(content), "otelcollector:")
	s.Require().Contains(string(content), "enabled: true")
}

// verifyDDOTServiceRemoved verifies DDOT service removal on Windows
func (s *extensionsSuite) verifyDDOTServiceRemoved() {
	serviceName := "datadog-otel-agent"
	output, err := s.Env().RemoteHost.Execute(`sc query "` + serviceName + `"`)
	// Service should not exist (error expected)
	s.Require().Error(err, "DDOT service should not exist after removal")
	s.Require().Contains(output, "does not exist", "Service should be deleted")
}
