// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// DisableDefender disables Windows Defender.
//
// NOTE: Microsoft recently deprecated/removed/disabled the registry keys that were used to disable Windows Defender.
// This means the WinDefend service will still be running, but it should not interfere (as much).
// https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/security-malware-windows-defender-disableantispyware
//
// TODO: Microsoft "recommends" to uninstall defender, but this only works on Windows Server and it requires a reboot.
func DisableDefender(host *components.RemoteHost) error {
	// check tamper protection status
	protected, err := IsTamperProtected(host)
	if err != nil {
		return err
	}
	if protected {
		return fmt.Errorf("Windows Defender is tamper protected, unable to modify settings")
	}

	_, err = powershell.PsHost().DisableWindowsDefender().Execute(host)
	if err != nil {
		return fmt.Errorf("error disabling Windows Defender: %w", err)
	}

	return nil
}

// IsTamperProtected returns true if Windows Defender is tamper protected.
// If true, then Windows Defender cannot be disabled programatically and must be
// disabled through the UI.
//
// https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/prevent-changes-to-security-settings-with-tamper-protection
//
// https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/manage-tamper-protection-individual-device
func IsTamperProtected(host *components.RemoteHost) (bool, error) {
	out, err := host.Execute("(Get-MpComputerStatus).IsTamperProtected")
	if err != nil {
		return false, fmt.Errorf("error checking if Windows Defender is tamper protected: %w", err)
	}
	out = strings.TrimSpace(out)
	return !strings.EqualFold(out, "False"), nil
}
