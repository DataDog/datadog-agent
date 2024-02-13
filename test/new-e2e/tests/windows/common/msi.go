// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package common contains helpers for Windows E2E tests
package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// MsiExec runs msiexec on the VM with the provided operation and args and collects the log
//
// args may need to be escaped/quoted. The Start-Process ArgumentList parameter value is wrapped in single quotes. For example:
//   - Start-Process -Wait msiexec -PassThru -ArgumentList '/qn /l "logfile" /i "msipath" APIKEY="00000000000000000000000000000000"'
//   - https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/start-process?view=powershell-7.4#example-7-specifying-arguments-to-the-process
//   - https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_quoting_rules?view=powershell-7.4
func MsiExec(host *components.RemoteHost, operation string, product string, args string, logPath string) error {
	remoteLogPath, err := GetTemporaryFile(host)
	if err != nil {
		return err
	}
	args = fmt.Sprintf(`/qn /l "%s" %s "%s" %s`, remoteLogPath, operation, product, args)
	cmd := fmt.Sprintf(`Exit (Start-Process -Wait msiexec -PassThru -ArgumentList '%s').ExitCode`, args)
	_, msiExecErr := host.Execute(cmd)
	// Always collect the log file, return error after
	if logPath != "" {
		err = host.GetFile(remoteLogPath, logPath)
		if err != nil {
			fmt.Printf("failed to collect install log: %s\n", err)
		}
	}

	return msiExecErr
}

// InstallMSI installs an MSI on the VM with the provided args and collects the install log
//
// args may need to be escaped/quoted, see MsiExec() for details
func InstallMSI(host *components.RemoteHost, msiPath string, args string, logPath string) error {
	err := MsiExec(host, "/i", msiPath, args, logPath)
	if err != nil {
		return fmt.Errorf("failed to install MSI: %w", err)
	}
	return nil
}

// UninstallMSI uninstalls an MSI on the VM and collects the uninstall log
func UninstallMSI(host *components.RemoteHost, msiPath string, logPath string) error {
	err := MsiExec(host, "/x", msiPath, "", logPath)
	if err != nil {
		return fmt.Errorf("failed to uninstall MSI: %w", err)
	}
	return nil
}

// RepairAllMSI repairs an MSI with /fa on the VM and collects the repair log
//
// /fa: a - forces all files to be reinstalled
//
// args may need to be escaped/quoted, see MsiExec() for details
func RepairAllMSI(host *components.RemoteHost, msiPath string, args string, logPath string) error {
	err := MsiExec(host, "/fa", msiPath, args, logPath)
	if err != nil {
		return fmt.Errorf("failed to repair MSI: %w", err)
	}
	return nil
}
