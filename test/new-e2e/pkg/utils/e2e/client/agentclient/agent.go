// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentclient provides an interface to run Agent commands.
package agentclient

// Agent is an interface to run Agent command.
type Agent interface {
	// Version runs version command returns the runtime Agent version
	Version(commandArgs ...AgentArgsOption) string

	// Hostname runs hostname command and returns the runtime Agent hostname
	Hostname(commandArgs ...AgentArgsOption) string

	// Config runs config command and returns the runtime agent config
	Config(commandArgs ...AgentArgsOption) string

	// ConfigWithError runs config command and returns the runtime agent config or an error
	ConfigWithError(commandArgs ...AgentArgsOption) (string, error)

	// Diagnose runs diagnose command and returns its output
	Diagnose(commandArgs ...AgentArgsOption) string

	// Flare runs flare command and returns the output. You should use the FakeIntake client to fetch the flare archive
	Flare(commandArgs ...AgentArgsOption) string

	// Health runs health command and returns the runtime agent health
	Health() (string, error)

	// ConfigCheck runs configcheck command and returns the runtime agent configcheck
	ConfigCheck(commandArgs ...AgentArgsOption) string

	// Integration run integration command and returns the output
	Integration(commandArgs ...AgentArgsOption) string

	// IntegrationWithError run integration command and returns the output
	IntegrationWithError(commandArgs ...AgentArgsOption) (string, error)

	// RemoteConfig runs the remote-config command and returns the output
	RemoteConfig(commandArgs ...AgentArgsOption) string

	// Secret runs the secret command
	Secret(commandArgs ...AgentArgsOption) string

	// IsReady runs status command and returns true if the command returns a zero exit code.
	// This function should rarely be used.
	IsReady() bool

	// Status runs status command and returns a Status struct
	Status(commandArgs ...AgentArgsOption) *Status

	// StatusWithError runs status command and returns a Status struct and error
	StatusWithError(commandArgs ...AgentArgsOption) (*Status, error)
}

// Status contains the Agent status content
type Status struct {
	Content string
}
