// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package driververifier

// Configuration represents the Windows NewDefender configuration
type Configuration struct {
	Enabled bool
	Target  string
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// EnabledOnDriver configures the DriverVerifier component to enable driver verifier on the specified driver.
func EnabledOnDriver(Target string) func(*Configuration) error {
	return func(p *Configuration) error {
		p.Enabled = true
		p.Target = Target
		return nil
	}
}

// Disabled configures the DriverVerifier component to disable driver verifier.
func Disabled() func(*Configuration) error {
	return func(p *Configuration) error {
		p.Enabled = false
		return nil
	}
}
