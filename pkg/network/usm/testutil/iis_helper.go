// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && test

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// getInstallIISScript reads the IIS installation script from E2E tests.
// This reuses the same script to avoid duplication.
func getInstallIISScript(t *testing.T) string {
	t.Helper()

	// Find the repository root by looking for go.mod
	cwd, err := os.Getwd()
	require.NoError(t, err)

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find repository root (go.mod)")
		}
		dir = parent
	}

	scriptPath := filepath.Join(dir, "test", "new-e2e", "tests", "windows", "scripts", "installiis.ps1")
	script, err := os.ReadFile(scriptPath)
	require.NoErrorf(t, err, "Failed to read IIS installation script from %s", scriptPath)

	return string(script)
}

// IISManager provides IIS management functionality for unit tests
type IISManager struct {
	t *testing.T
}

// NewIISManager creates a new IISManager
func NewIISManager(t *testing.T) *IISManager {
	return &IISManager{t: t}
}

// IsIISInstalled checks if IIS is installed
func (m *IISManager) IsIISInstalled() bool {
	m.t.Helper()

	// Try Windows Server method first
	cmd := exec.Command("powershell", "-Command", "(Get-WindowsFeature -Name Web-Server -ErrorAction SilentlyContinue).InstallState -eq 'Installed'")
	output, err := cmd.CombinedOutput()
	if err == nil && strings.TrimSpace(string(output)) == "True" {
		return true
	}

	// Try Windows Client method (Windows 10/11)
	cmd = exec.Command("powershell", "-Command", "(Get-WindowsOptionalFeature -Online -FeatureName IIS-WebServer -ErrorAction SilentlyContinue).State -eq 'Enabled'")
	output, err = cmd.CombinedOutput()
	if err != nil {
		m.t.Logf("Failed to check IIS installation status: %v", err)
		return false
	}

	return strings.TrimSpace(string(output)) == "True"
}

// IsWindowsServer checks if the system is Windows Server
func (m *IISManager) IsWindowsServer() bool {
	m.t.Helper()
	cmd := exec.Command("powershell", "-Command", "(Get-CimInstance Win32_OperatingSystem).Caption -like '*Server*'")
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.t.Logf("Failed to detect Windows version: %v", err)
		return false
	}
	return strings.TrimSpace(string(output)) == "True"
}

// InstallIIS installs IIS on the system
func (m *IISManager) InstallIIS() error {
	m.t.Helper()
	m.t.Log("Installing IIS Web-Server feature (this may take several minutes)...")

	var script string
	if m.IsWindowsServer() {
		// Windows Server: Use the E2E test script
		m.t.Log("Detected Windows Server, using Install-WindowsFeature...")
		script = getInstallIISScript(m.t)
	} else {
		// Windows Client (10/11): Use Enable-WindowsOptionalFeature
		m.t.Log("Detected Windows Client, using Enable-WindowsOptionalFeature...")
		script = `
$result = Enable-WindowsOptionalFeature -Online -FeatureName IIS-WebServer -All -NoRestart
if ($result.RestartNeeded) {
    Write-Output "RESTART_REQUIRED"
    exit 3010
}
Write-Output "IIS installed successfully"
exit 0
`
	}

	cmd := exec.Command("powershell", "-Command", script)
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

		if !m.IsIISInstalled() {
			m.t.Fatal("IIS installation completed but IIS is still not available")
		}
		m.t.Log("IIS installation verified")
	}
}

// SetupIISSite creates and starts an IIS site
func (m *IISManager) SetupIISSite(siteName string, port int, indexContent string) error {
	m.t.Helper()

	siteDir := "C:\\inetpub\\wwwroot\\" + siteName

	script := fmt.Sprintf(`
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
`, siteName, siteDir, port, indexContent)

	cmd := exec.Command("powershell", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to setup IIS site: %w, output: %s", err, string(output))
	}

	m.t.Logf("IIS site created: %s on port %d", siteName, port)
	return nil
}

// CleanupIISSite removes an IIS site
func (m *IISManager) CleanupIISSite(siteName string) error {
	m.t.Helper()

	siteDir := "C:\\inetpub\\wwwroot\\" + siteName

	script := fmt.Sprintf(`
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
`, siteName, siteDir)

	cmd := exec.Command("powershell", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.t.Logf("Warning: Failed to cleanup IIS site: %v, output: %s", err, string(output))
		return fmt.Errorf("failed to cleanup IIS site: %w", err)
	}

	m.t.Logf("IIS site cleaned up: %s", siteName)
	return nil
}
