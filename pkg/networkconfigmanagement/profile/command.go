// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package profile defines models, logic, functions to load/parse/manage network device profiles
package profile

import (
	"fmt"
	"regexp"
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

type Command interface {
	CommandType() string
}

// PlainCommand represents a single command plus zero or more regexes to run against
// the combined stdout/stderr of that command.
type PlainCommand struct {
	Command   string    `json:"command"`
	Validator Validator `json:"validator"`
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
