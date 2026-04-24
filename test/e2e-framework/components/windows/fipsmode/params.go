// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package fipsmode

// Configuration represents the Windows FIPS mode configuration
type Configuration struct {
	FIPSModeEnabled bool
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithFIPSModeEnabled configures the FIPSMode component to enable FIPS mode on the Windows host
//
// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/security-policy-settings/system-cryptography-use-fips-compliant-algorithms-for-encryption-hashing-and-signing
func WithFIPSModeEnabled() func(*Configuration) error {
	return func(p *Configuration) error {
		p.FIPSModeEnabled = true
		return nil
	}
}
