// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package common contains helpers for Windows E2E tests
package common

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v7"
	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// Windows Installer exit codes. See:
// https://learn.microsoft.com/en-us/windows/win32/msi/error-codes
const (
	msiExitSuccessRebootRequired    = 3010 // ERROR_SUCCESS_REBOOT_REQUIRED
	msiExitSuccessRebootInitiated   = 1641 // ERROR_SUCCESS_REBOOT_INITIATED
	msiExitInstallPackageOpenFailed = 1619 // ERROR_INSTALL_PACKAGE_OPEN_FAILED
	msiExitInstallPackageInvalid    = 1620 // ERROR_INSTALL_PACKAGE_INVALID
)

// msiTransientPackageFailureExitCodes are msiexec exit codes that indicate the package could
// not be opened or read. These are typically transient when the package is fetched over the
// network, so they're retried. Add new transient codes here.
var msiTransientPackageFailureExitCodes = []int{
	msiExitInstallPackageOpenFailed,
	msiExitInstallPackageInvalid,
}

func isMsiSuccessExitCode(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	code := exitErr.ExitStatus()
	return code == msiExitSuccessRebootRequired || code == msiExitSuccessRebootInitiated
}

// isMsiTransientPackageFailureExitCode reports whether err is an msiexec failure with one of the
// transient package-failure exit codes in msiTransientPackageFailureExitCodes.
func isMsiTransientPackageFailureExitCode(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return slices.Contains(msiTransientPackageFailureExitCodes, exitErr.ExitStatus())
}

func isRemoteMSIPath(p string) bool {
	return strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://")
}

// MsiExec runs msiexec on the VM with the provided operation and args and collects the log
//
// args may need to be escaped/quoted. The Start-Process ArgumentList parameter value is wrapped in single quotes. For example:
//   - Start-Process -Wait msiexec -PassThru -ArgumentList '/qn /norestart /l "logfile" /i "msipath" APIKEY="00000000000000000000000000000000"'
//   - https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/start-process?view=powershell-7.4#example-7-specifying-arguments-to-the-process
//   - https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_quoting_rules?view=powershell-7.4
func MsiExec(host *components.RemoteHost, operation string, product string, args string, logPath string) error {

	if !strings.HasPrefix(operation, "/") {
		return fmt.Errorf("unexpected operation: %s", operation)
	}

	remoteLogPath, err := GetTemporaryFile(host)
	if err != nil {
		return err
	}
	args = fmt.Sprintf(`/qn /norestart /l "%s" %s "%s" %s`, remoteLogPath, operation, product, args)
	cmd := fmt.Sprintf(`Exit (Start-Process -Wait msiexec -PassThru -ArgumentList '%s').ExitCode`, args)

	_, msiExecErr := backoff.Retry(context.Background(), func() (any, error) {
		_, err := host.Execute(cmd)
		if err == nil || isMsiSuccessExitCode(err) {
			return nil, nil // Treat reboot-required exit codes as success
		}
		// Retry transient package failures (exit 1619 ERROR_INSTALL_PACKAGE_OPEN_FAILED or
		// 1620 ERROR_INSTALL_PACKAGE_INVALID) when installing from a remote URL. We've seen
		// S3-hosted MSIs fail to open or download momentarily mid-test even when the same URL
		// succeeded earlier in the same run (e.g. WINA-2296, WINA-2869). These codes fire before
		// msiexec writes any action to the log, so they're a clean signal that nothing actually started.
		if isMsiTransientPackageFailureExitCode(err) && operation == "/i" && isRemoteMSIPath(product) {
			fmt.Printf("msiexec /i %s failed with a transient package error, retrying\n", product)
			return nil, err
		}
		// Fail on any other error
		return nil, backoff.Permanent(err)
	}, backoff.WithBackOff(backoff.NewConstantBackOff(5*time.Second)), backoff.WithMaxTries(3))
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
func UninstallMSI(host *components.RemoteHost, msiPath string, args string, logPath string) error {
	err := MsiExec(host, "/x", msiPath, args, logPath)
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
