// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defender

// Configuration represents the Windows NewDefender configuration
type Configuration struct {
	Disabled  bool
	Uninstall bool
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithDefenderDisabled configures the NewDefender component to disable Windows NewDefender
func WithDefenderDisabled() func(*Configuration) error {
	return func(p *Configuration) error {
		p.Disabled = true
		return nil
	}
}

// WithDefenderUninstalled configures the NewDefender component to uninstall Windows NewDefender
func WithDefenderUninstalled() func(*Configuration) error {
	return func(p *Configuration) error {
		p.Uninstall = true
		return nil
	}
}
