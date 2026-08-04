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

// DefaultEnablePasswordPrompt is used to recognize an enable command's
// password prompt when EnableCommand.PasswordPrompt is unset.
var DefaultEnablePasswordPrompt = regexp.MustCompile(`(?i)password:\s*$`)

// EnableCommand represents the command used to elevate to a device's
// privileged/enable mode before running the rest of a profile's commands.
// PasswordPrompt is the regex used to recognize the device's password
// prompt in the session output; Validator (optional) checks the response
// after the password is sent, to confirm enable actually succeeded before
// proceeding to run the real command.
type EnableCommand struct {
	Command        string         `json:"command"`
	PasswordPrompt *regexp.Regexp `json:"password_prompt,omitempty"`
	Validator      Validator      `json:"validator,omitempty"`
}

func (c *EnableCommand) CommandType() string {
	return "enable"
}

// EffectivePasswordPrompt returns c.PasswordPrompt, or
// DefaultEnablePasswordPrompt if c is nil or PasswordPrompt is unset.
func (c *EnableCommand) EffectivePasswordPrompt() *regexp.Regexp {
	if c != nil && c.PasswordPrompt != nil {
		return c.PasswordPrompt
	}
	return DefaultEnablePasswordPrompt
}
