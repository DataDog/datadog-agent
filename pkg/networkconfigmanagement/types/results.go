// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"errors"
	"fmt"
)

// CommandResult records a command that was run and the resulting output.
type CommandResult struct {
	CommandStr string `json:"command"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
}

// FormattedError returns nil if there was no error, and otherwise returns an
// error containing .Error and, if it was nonempty, .Output.
func (c *CommandResult) FormattedError() error {
	if c.Error == "" {
		return nil
	}
	if c.Output != "" {
		return fmt.Errorf("%v: %q", c.Error, c.Output)
	}
	return errors.New(c.Error)
}

// ResultList is a list of CommandResults
type ResultList []*CommandResult

// PushResult captures the possible outcomes of executing a PushConfig. We need
// a more complex structure than just a simple error because we want to track
// what commands were executed and which ones completed successfully - if, for
// example, a config push successfully copies the configuration to the device
// and sets the running config but fails to set the startup config, it's
// important that the calling code know that the running configuration has been
// changed even though the full config replace operation failed.
type PushResult struct {
	// CopyConfig holds the CommandResults of copying the configuration to the
	// device (generally via SCP)
	CopyConfig ResultList `json:"copy_config"`
	// SetRunning holds the results of attempting to set the device's running
	// config to the copied configuration.
	SetRunning ResultList `json:"set_running"`
	// SetStartup holds the results of attempting to set the device's startup
	// config to the copied configuration. In most profiles this is done by
	// copying the running config to the startup config, so this will be empty
	// if SetRunning contains errors
	SetStartup ResultList `json:"set_startup"`
}

// RollbackResponse is the JSON body returned by the /agent/ncm/rollback endpoint.
type RollbackResponse struct {
	CommandResults *PushResult `json:"command_results"`
	ErrorCode      string      `json:"error_code,omitempty"`
	ErrorMsg       string      `json:"error_msg,omitempty"`
}

// RunCommandResponse is the JSON body returned by the /agent/ncm/run-command endpoint.
type RunCommandResponse struct {
	CommandResult *CommandResult `json:"result"`
	ErrorCode     string         `json:"error_code,omitempty"`
	ErrorMsg      string         `json:"error_msg,omitempty"`
}
