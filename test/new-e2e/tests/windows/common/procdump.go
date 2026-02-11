// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

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

// ProcdumpSession wraps an SSH session running procdump.
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

// StartProcdump starts procdump in the background
func StartProcdump(host *components.RemoteHost, processName string) (*ProcdumpSession, error) {

	// Start procdump:
	// -accepteula: Accept the EULA automatically
	// -ma: Write a full dump file
	// -n 5: Write 5 dumps
	// -s 5: Wait 5 seconds between dumps
	// -w: Wait for the specified process to launch if it's not running
	cmd := fmt.Sprintf(`& "%s" -accepteula -ma -n 5 -s 5 -w %s "%s"`, ProcdumpExe, processName, ProcdumpsPath)

	session, _, _, err := host.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start procdump: %w", err)
	}

	return &ProcdumpSession{Session: session}, nil
}
