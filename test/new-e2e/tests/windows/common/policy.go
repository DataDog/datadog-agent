// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// setFIPSAlgorithmPolicy configures local security policy to enable or disable FIPS mode.
//
// The setting is applied system-wide and does NOT require a reboot.
// This setting may be overridden by group policy.
//
// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/security-policy-settings/system-cryptography-use-fips-compliant-algorithms-for-encryption-hashing-and-signing
func setFIPSAlgorithmPolicy(host *components.RemoteHost, enabled bool) error {
	path := `HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FIPSAlgorithmPolicy`
	valueName := "Enabled"
	value := 0
	if enabled {
		value = 1
	}
	return SetRegistryDWORDValue(host, path, valueName, value)
}

// EnableFIPSMode enables FIPS mode on the host.
//
// The setting is applied system-wide and does NOT require a reboot.
// This setting may be overridden by group policy.
//
// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/security-policy-settings/system-cryptography-use-fips-compliant-algorithms-for-encryption-hashing-and-signing
func EnableFIPSMode(host *components.RemoteHost) error {
	return setFIPSAlgorithmPolicy(host, true)
}

// DisableFIPSMode disables FIPS mode on the host.
//
// The setting is applied system-wide and does NOT require a reboot.
// This setting may be overridden by group policy.
//
// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/security-policy-settings/system-cryptography-use-fips-compliant-algorithms-for-encryption-hashing-and-signing
func DisableFIPSMode(host *components.RemoteHost) error {
	return setFIPSAlgorithmPolicy(host, false)
}
