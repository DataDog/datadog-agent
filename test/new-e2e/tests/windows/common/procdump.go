// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	// ProcdumpPath is the directory where procdump is installed
	ProcdumpPath = "C:/procdump"
	// ProcdumpExe is the path to the procdump executable
	ProcdumpExe = "C:/procdump/procdump.exe"
	// ProcdumpZipPath is the path where the procdump zip file is downloaded
	ProcdumpZipPath = "C:/procdump.zip"
	// ProcdumpDownloadURL is the URL to download procdump from Sysinternals
	ProcdumpDownloadURL = "https://download.sysinternals.com/files/Procdump.zip"
	// ProcdumpsPath is the directory where procdump captures are stored.
	ProcdumpsPath = "C:/procdumps"
)

// SetupProcdump downloads and extracts procdump to the remote host if not already present.
func SetupProcdump(host *components.RemoteHost) error {
	err := host.HostArtifactClient.Get("windows-products/Procdump.zip", ProcdumpZipPath)
	if err != nil {
		return fmt.Errorf("failed to download procdump: %w", err)
	}

	_, err = host.Execute(fmt.Sprintf(`if (-Not (Test-Path -Path '%s')) { Expand-Archive -Path '%s' -DestinationPath '%s' }`, ProcdumpPath, ProcdumpZipPath, ProcdumpPath))
	if err != nil {
		return fmt.Errorf("failed to setup procdump: %w", err)
	}

	// Create the procdump output directory (separate from WER dumps)
	_, err = host.Execute(fmt.Sprintf(`New-Item -ItemType Directory -Path '%s' -Force`, ProcdumpsPath))
	if err != nil {
		return fmt.Errorf("failed to create procdump output directory: %w", err)
	}

	return nil
}

// CaptureProcdump captures a full memory dump of a process by PID
func CaptureProcdump(host *components.RemoteHost, pid int, outputDir string, processName string) (string, error) {
	dumpFileName := fmt.Sprintf("%s.%d.dmp", processName, pid)
	dumpPath := filepath.Join(outputDir, dumpFileName)
	// Use forward slashes for PowerShell compatibility
	dumpPath = strings.ReplaceAll(dumpPath, "\\", "/")

	cmd := fmt.Sprintf(`& "%s" -accepteula -ma %d "%s"`, ProcdumpExe, pid, dumpPath)
	output, err := host.Execute(cmd)

	// Procdump returns exit code 1 when "Dump count reached", which is success.
	// Check both output and error message to determine if the dump was captured.
	// When host.Execute returns an error, the command output may be embedded in
	// the error message rather than the output string.
	successIndicator := "Dump 1 complete"
	if strings.Contains(output, successIndicator) {
		// Dump was successful, ignore exit code
		return dumpPath, nil
	}
	if err != nil && strings.Contains(err.Error(), successIndicator) {
		// Dump was successful but output was in error message, ignore exit code
		return dumpPath, nil
	}

	// If we don't see success in output or error and there was an error, report it
	if err != nil {
		return "", fmt.Errorf("procdump failed for PID %d: %w\nOutput: %s", pid, err, output)
	}

	// No error but also no success message - unexpected
	return "", fmt.Errorf("procdump did not capture dump for PID %d, output: %s", pid, output)
}

// ProcdumpSession wraps an SSH session running procdump.
// Use Close() to terminate procdump when no longer needed.
type ProcdumpSession struct {
	Session *ssh.Session
}

// Close terminates the procdump process if it's still running.
func (ps *ProcdumpSession) Close() {
	if ps.Session != nil {
		_ = ps.Session.Close()
		ps.Session = nil
	}
}

// StartProcdump starts procdump in the background, waiting for the specified process
// to launch. Procdump will capture a full memory dump when the process terminates.
func StartProcdump(host *components.RemoteHost, processName, outputDir string) (*ProcdumpSession, error) {
	// Build the dump path
	dumpPath := filepath.Join(outputDir, processName)
	// Use forward slashes for PowerShell compatibility
	dumpPath = strings.ReplaceAll(dumpPath, "\\", "/")

	// Start procdump:
	// -accepteula: Accept the EULA automatically
	// -ma: Write a full dump file
	// -t: Write a dump when the process terminates
	// -w: Wait for the specified process to launch if it's not running
	cmd := fmt.Sprintf(`& "%s" -accepteula -ma -t -w %s "%s"`, ProcdumpExe, processName, dumpPath)

	session, _, _, err := host.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start procdump: %w", err)
	}

	return &ProcdumpSession{Session: session}, nil
}
