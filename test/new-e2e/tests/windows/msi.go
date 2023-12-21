// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package windows contains helpers for Windows E2E tests
package windows

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// InstallMSI installs an MSI on the VM with the provided args and collects the install log
func InstallMSI(host *components.RemoteHost, msiPath string, args string, logPath string) error {
	remoteLogPath, err := GetTemporaryFile(host)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`Start-Process -Wait msiexec -PassThru -ArgumentList '/qn /l %s /i %s %s'`,
		remoteLogPath, msiPath, args)

	output, installErr := host.Execute(cmd)
	// Collect the install log
	err = host.GetFile(remoteLogPath, logPath)
	if err != nil {
		fmt.Printf("failed to collect install log: %s\n", err)
	}
	if installErr != nil {
		return fmt.Errorf("failed to install MSI: %w\n%s", installErr, output)
	}
	return nil
}

// UninstallMSI uninstalls an MSI on the VM and collects the uninstall log
func UninstallMSI(host *components.RemoteHost, msiPath string, logPath string) error {
	remoteLogPath, err := GetTemporaryFile(host)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("Exit (start-process -passthru -wait msiexec.exe -argumentList /x,'%s',/qn,/l,%s).ExitCode", msiPath, remoteLogPath)

	output, uninstallerr := host.Execute(cmd)
	// Collect the install log
	err = host.GetFile(remoteLogPath, logPath)
	if err != nil {
		fmt.Printf("failed to collect uninstall log: %s\n", err)
	}

	if uninstallerr != nil {
		return fmt.Errorf("failed to uninstall MSI: %w\n%s", uninstallerr, output)
	}
	return nil
}
