// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultTimeoutSeconds  int = 30
	defaultOutputSizeLimit int = 2 * 1024
)

type commandCheck struct {
	baseCheck
	command        *compliance.Command
	execCommand    compliance.BinaryCmd
	commandTimeout time.Duration
	maxOutputSize  int
}

func newCommandCheck(baseCheck baseCheck, command *compliance.Command) (*commandCheck, error) {
	if command.BinaryCmd == nil && command.ShellCmd == nil {
		return nil, fmt.Errorf("unable to create commandCheck - need a binary or a shell command")
	}

	commandCheck := commandCheck{
		baseCheck: baseCheck,
		command:   command,
	}

	// Create `execCommand` from `command` model
	// Binary takes precedence over Shell
	if command.BinaryCmd != nil {
		commandCheck.execCommand = *command.BinaryCmd
	} else {
		if command.ShellCmd.Shell != nil {
			commandCheck.execCommand = *command.ShellCmd.Shell
		} else {
			commandCheck.execCommand = getDefaultShell()
		}

		commandCheck.execCommand.Args = append(commandCheck.execCommand.Args, command.ShellCmd.Run)
	}

	if command.TimeoutSeconds != 0 {
		commandCheck.commandTimeout = time.Duration(command.TimeoutSeconds) * time.Second
	} else {
		commandCheck.commandTimeout = time.Duration(defaultTimeoutSeconds) * time.Second
	}

	if command.MaxOutputSize != 0 {
		commandCheck.maxOutputSize = command.MaxOutputSize
	} else {
		commandCheck.maxOutputSize = defaultOutputSizeLimit
	}

	return &commandCheck, nil
}

func (c *commandCheck) Run() error {
	log.Debugf("%s: running command check: %v", c.id, c.command)

	context, cancel := context.WithTimeout(context.Background(), c.commandTimeout)
	defer cancel()

	// TODO: Capture stdout only when necessary
	exitCode, stdout, err := commandRunnerFunc(context, c.execCommand.Name, c.execCommand.Args, true)
	if exitCode == -1 && err != nil {
		return log.Warnf("%s: command '%v' execution failed, error: %v", c.id, c.command, err)
	}

	var shouldReport = false
	if len(c.command.Filter) == 0 {
		shouldReport = true
	} else {
		for _, filter := range c.command.Filter {
			if filter.Include != nil && filter.Include.ExitCode == exitCode {
				shouldReport = true
				break
			}
			if filter.Exclude != nil && filter.Exclude.ExitCode == exitCode {
				break
			}
		}
	}

	if shouldReport {
		return c.reportCommand(exitCode, stdout)
	}

	return log.Warnf("%s: command '%v' returned with exitcode: %d (not reportable), error: %v", c.id, c.command, exitCode, err)
}

func (c *commandCheck) reportCommand(exitCode int, stdout []byte) error {
	if len(stdout) > c.maxOutputSize {
		return log.Errorf("%s: command '%v' output is too large: %d, won't be reported", c.id, c.command, len(stdout))
	}

	kv := compliance.KVMap{
		"exitCode": strconv.Itoa(exitCode),
	}
	strStdout := string(stdout)

	for _, field := range c.command.Report {
		if len(field.As) > 0 {
			kv[field.As] = strStdout
		}
	}

	c.report(nil, kv)
	return nil
}
