// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test && npm

package testutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// Windows Client IIS installation script
	windowsClientInstallScript = `
$result = Enable-WindowsOptionalFeature -Online -FeatureName IIS-WebServer -All -NoRestart
if ($result.RestartNeeded) {
    Write-Output "RESTART_REQUIRED"
    exit 3010
}
Write-Output "IIS installed successfully"
exit 0
`

	// IIS site setup script template
	// Parameters: siteName, siteDir, port, indexContent
	setupIISSiteScript = `
$SiteName = "%s"
$SitePath = "%s"
$Port = %d

# Create directory
New-Item -ItemType Directory -Force -Path $SitePath | Out-Null

# Create index.html
Set-Content -Path "$SitePath\index.html" -Value @'
%s
'@

# Create the IIS site
New-WebSite -Name $SiteName -PhysicalPath $SitePath -Port $Port -Force | Out-Null

# Start the site
Start-WebSite -Name $SiteName

# Wait for site to be ready
Start-Sleep -Seconds 2

Write-Output "Site created successfully"
`

	// IIS site cleanup script template
	// Parameters: siteName, siteDir
	cleanupIISSiteScript = `
$SiteName = "%s"
$SitePath = "%s"

# Stop and remove the site
if (Get-WebSite -Name $SiteName -ErrorAction SilentlyContinue) {
    Stop-WebSite -Name $SiteName -ErrorAction SilentlyContinue
    Remove-WebSite -Name $SiteName -ErrorAction SilentlyContinue
}

# Remove the directory
if (Test-Path $SitePath) {
    Remove-Item -Path $SitePath -Recurse -Force -ErrorAction SilentlyContinue
}
`
)

// IISManager provides IIS management functionality for unit tests
type IISManager struct {
	t               *testing.T
	isWindowsServer func() bool // Lazy-initialized OS detection using sync.OnceValue
}

// NewIISManager creates a new IISManager
func NewIISManager(t *testing.T) *IISManager {
	m := &IISManager{t: t}
	// Initialize the sync.OnceValue wrapper for OS detection
	m.isWindowsServer = sync.OnceValue(func() bool {
		result := m.checkPowerShellBool("(Get-CimInstance Win32_OperatingSystem).Caption -like '*Server*'")
		if !result {
			t.Log("Detected Windows Client")
		} else {
			t.Log("Detected Windows Server")
		}
		return result
	})
	return m
}

// runPowerShell executes a PowerShell command and returns the output
func (m *IISManager) runPowerShell(command string) (string, error) {
	cmd := exec.Command("powershell", "-Command", command)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// findRepoRoot finds the repository root by looking for go.mod
func (m *IISManager) findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root not found")
		}
		dir = parent
	}
}

// checkPowerShellBool runs a PowerShell command and checks if result is "True"
func (m *IISManager) checkPowerShellBool(command string) bool {
	output, err := m.runPowerShell(command)
	if err != nil {
		m.t.Logf("PowerShell command failed: %v, output: %s", err, output)
		return false
	}
	return output == "True"
}

// getSiteDir returns the directory path for an IIS site
func (m *IISManager) getSiteDir(siteName string) string {
	return filepath.Join("C:\\", "inetpub", "wwwroot", siteName)
}

// IsIISInstalled checks if IIS is installed
func (m *IISManager) IsIISInstalled() bool {
	m.t.Helper()

	var command string
	if m.IsWindowsServer() {
		// Windows Server method
		command = "(Get-WindowsFeature -Name Web-Server -ErrorAction SilentlyContinue).InstallState -eq 'Installed'"
	} else {
		// Windows Client method (Windows 10/11)
		command = "(Get-WindowsOptionalFeature -Online -FeatureName IIS-WebServer -ErrorAction SilentlyContinue).State -eq 'Enabled'"
	}

	return m.checkPowerShellBool(command)
}

// IsWindowsServer checks if the system is Windows Server
func (m *IISManager) IsWindowsServer() bool {
	m.t.Helper()
	return m.isWindowsServer()
}

// InstallIIS installs IIS on the system
func (m *IISManager) InstallIIS() error {
	m.t.Helper()
	m.t.Log("Installing IIS Web-Server feature (this may take several minutes)...")

	var cmd *exec.Cmd
	if m.IsWindowsServer() {
		// Windows Server: Use the original E2E test script
		repoRoot, err := m.findRepoRoot()
		if err != nil {
			return fmt.Errorf("failed to find repository root: %w", err)
		}
		scriptPath := filepath.Join(repoRoot, "test", "new-e2e", "tests", "windows", "scripts", "installiis.ps1")

		// Verify the script exists
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return fmt.Errorf("IIS installation script not found at %s", scriptPath)
		}

		cmd = exec.Command("powershell", "-File", scriptPath)
	} else {
		// Windows Client (10/11): Use Enable-WindowsOptionalFeature
		cmd = exec.Command("powershell", "-Command", windowsClientInstallScript)
	}

	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	if err != nil {
		// Check if exit code is 3010 (restart required)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3010 {
			m.t.Skipf("IIS installed but system restart is required: %s", outputStr)
		}
		return fmt.Errorf("failed to install IIS: %w, output: %s", err, outputStr)
	}

	m.t.Log("IIS installed successfully")
	return nil
}

// EnsureIISInstalled checks if IIS is installed and installs it if not
func (m *IISManager) EnsureIISInstalled() {
	m.t.Helper()

	if !m.IsIISInstalled() {
		m.t.Log("IIS is not installed, installing...")
		err := m.InstallIIS()
		require.NoError(m.t, err, "Failed to install IIS")

		require.Truef(m.t, m.IsIISInstalled(), "IIS installation completed but IIS is still not available")
		m.t.Log("IIS installation verified")
	} else {
		m.t.Log("IIS is already installed")
	}
}

// SetupIISSite creates and starts an IIS site
func (m *IISManager) SetupIISSite(siteName string, port int, indexContent string) error {
	m.t.Helper()

	siteDir := m.getSiteDir(siteName)
	script := fmt.Sprintf(setupIISSiteScript, siteName, siteDir, port, indexContent)

	output, err := m.runPowerShell(script)
	if err != nil {
		return fmt.Errorf("failed to setup IIS site: %w, output: %s", err, output)
	}

	m.t.Logf("IIS site created: %s on port %d", siteName, port)
	return nil
}

// CleanupIISSite removes an IIS site
func (m *IISManager) CleanupIISSite(siteName string) error {
	m.t.Helper()

	siteDir := m.getSiteDir(siteName)
	script := fmt.Sprintf(cleanupIISSiteScript, siteName, siteDir)

	output, err := m.runPowerShell(script)
	if err != nil {
		m.t.Logf("Warning: Failed to cleanup IIS site: %v, output: %s", err, output)
		return fmt.Errorf("failed to cleanup IIS site: %w", err)
	}

	m.t.Logf("IIS site cleaned up: %s", siteName)
	return nil
}
