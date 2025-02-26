// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains the different types used by the system-probe config.
//
// This types are extracted to their own module so other can link against it without compiling with the entire
// system-probe code base.
package types

// ModuleName is a typed alias for string, used only for module names
type ModuleName string

// Config represents the configuration options for the system-probe
type Config struct {
	EnabledModules map[ModuleName]struct{}

	SocketAddress string

	LogFile  string
	LogLevel string

	StatsdHost         string
	MaxConnsPerMessage int

	DebugPort  int
	HealthPort int
	StatsdPort int
	Enabled    bool

	// When the system-probe is enabled in a separate container, we need a way to also disable the system-probe
	// packaged in the main agent container (without disabling network collection on the process-agent).
	ExternalSystemProbe bool

	TelemetryEnabled bool
}

// ModuleIsEnabled returns a bool indicating if the given module name is enabled.
func (c Config) ModuleIsEnabled(modName ModuleName) bool {
	_, ok := c.EnabledModules[modName]
	return ok
}
