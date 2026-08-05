// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package profile defines models, logic, functions to load/parse/manage network device profiles
package profile

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// Validator contains rules for validating the output of a command - requiring
// specific regexes to be present or absent in stdout and/or stderr.
type Validator struct {
	Require []*regexp.Regexp `json:"require,omitempty"`
	Reject  []*regexp.Regexp `json:"reject,omitempty"`
}

func (v *Validator) Validate(text string) error {
	for _, rule := range v.Require {
		if !rule.MatchString(text) {
			return fmt.Errorf("does not match required regex %q", rule)
		}
	}
	for _, rule := range v.Reject {
		if rule.MatchString(text) {
			return fmt.Errorf("matches failure regex %q", rule)
		}
	}
	return nil
}

// ValidateResult is a no-op if c.Error is already set, otherwise it runs v on
// c.Output and saves the result in c.Error.
func (v *Validator) ValidateResult(c *types.CommandResult) {
	if c.Error != "" {
		return
	}
	if err := v.Validate(c.Output); err != nil {
		c.Error = err.Error()
	}
}

type Command interface {
	CommandType() string
}

// PlainCommand represents a single command plus zero or more regexes to run against
// the combined stdout/stderr of that command.
type PlainCommand struct {
	Command   string    `json:"command"`
	Validator Validator `json:"validator"`
	// SetupCommands run before Command in the same exec session. Note: if a setup
	// command prints output, it may appear in the saved config.
	SetupCommands []string `json:"setup_commands,omitempty"`
	// Interactive, when true, runs Command over an interactive PTY shell session
	// rather than a one-shot exec. Some devices (notably PAN-OS) only emit
	// command output on an interactive TTY; a non-interactive exec returns just
	// the login banner. When Interactive is set, Prompt must be set too, and
	// SetupCommands are sent one at a time (each waiting for the prompt) before
	// Command.
	Interactive bool `json:"interactive,omitempty"`
	// Prompt matches the device's interactive CLI prompt. It is used to detect
	// when the device is ready for input and when a command has finished. It
	// must be anchored tightly enough not to match config content (e.g. anchor
	// on the "user@host>" shape rather than any line ending in ">"). Required
	// when Interactive is true.
	Prompt *regexp.Regexp `json:"-"`
}

func (c *PlainCommand) CommandType() string {
	return "plain"
}

// SCPCommand represents a command that expects to receive valid scp input via
// stdin. The actual command run over SSH will be `<RemoteCommand> -t <FilePath>`
type SCPCommand struct {
	RemoteCommand string `json:"remote_command"`
	Filepath      string `json:"filepath"`
	// usually this should be empty - scp does not print output on most systems.
	Validator Validator `json:"validator"`
}

func (c *SCPCommand) CommandType() string {
	return "scp"
}
