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

	// Check runs check command and returns the runtime Agent check
	Check(commandArgs ...AgentArgsOption) string

	// Check runs check command and returns the runtime Agent check or an error
	CheckWithError(commandArgs ...AgentArgsOption) (string, error)

	// Config runs config command and returns the runtime agent config
	Config(commandArgs ...AgentArgsOption) string

	// ConfigWithError runs config command and returns the runtime agent config or an error
	ConfigWithError(commandArgs ...AgentArgsOption) (string, error)

	// Diagnose runs diagnose command and returns its output
	Diagnose(commandArgs ...AgentArgsOption) string

	// Flare runs flare command and returns the output. You should use the FakeIntake client to fetch the flare archive
	Flare(commandArgs ...AgentArgsOption) string

	// FlareWithError runs flare command and returns the output and error, if any. You should use the FakeIntake client to fetch the flare archive
	FlareWithError(commandArgs ...AgentArgsOption) (string, error)

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

	// Restart restarts the Agent service and returns an error if the restart command fails.
	// After a successful call the agent is not yet ready; callers should poll IsReady if needed.
	Restart() error

	// Status runs status command and returns a Status struct
	Status(commandArgs ...AgentArgsOption) *Status

	// StatusWithError runs status command and returns a Status struct and error
	StatusWithError(commandArgs ...AgentArgsOption) (*Status, error)

	// JMX run the jmx command and returns a Status struct and error
	JMX(commandArgs ...AgentArgsOption) (*Status, error)

	// WorkloadList runs the workload-list command and returns the output
	WorkloadList() (*Status, error)
}

// Status contains the Agent status content
type Status struct {
	Content string
}

// DiagnoseCounters contains the count of diagnosis results from a diagnose run.
type DiagnoseCounters struct {
	Total         int `json:"total,omitempty"`
	Success       int `json:"success,omitempty"`
	Fail          int `json:"fail,omitempty"`
	Warnings      int `json:"warnings,omitempty"`
	UnexpectedErr int `json:"unexpected_error,omitempty"`
}

// DiagnoseEntry is a single diagnosis entry within a suite run.
type DiagnoseEntry struct {
	Name      string `json:"name"`
	Status    string `json:"result"`
	Diagnosis string `json:"diagnosis"`
	Category  string `json:"category,omitempty"`
}

// DiagnoseRun holds one suite's results inside a diagnose.Result payload.
type DiagnoseRun struct {
	SuiteName string          `json:"suite_name"`
	Diagnoses []DiagnoseEntry `json:"diagnoses"`
}

// DiagnoseResult is the top-level JSON structure returned by `agent diagnose --json`.
// It mirrors comp/core/diagnose/def.Result.
type DiagnoseResult struct {
	Runs    []DiagnoseRun    `json:"runs"`
	Summary DiagnoseCounters `json:"summary"`
}
