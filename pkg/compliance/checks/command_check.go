// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultTimeout = 30 * time.Second
)

var commandReportedFields = []string{
	compliance.CommandFieldExitCode,
}

func resolveCommand(ctx context.Context, _ env.Env, ruleID string, res compliance.BaseResource) (resolved, error) {
	if res.Command == nil {
		return nil, fmt.Errorf("%s: expecting command resource in command check", ruleID)
	}

	command := res.Command

	log.Debugf("%s: running command check: %v", ruleID, command)

	if command.BinaryCmd == nil && command.ShellCmd == nil {
		return nil, fmt.Errorf("unable to execute commandCheck - need a binary or a shell command")
	}

	var execCommand = command.BinaryCmd

	// Create `execCommand` from `command` model
	// Binary takes precedence over Shell
	if execCommand == nil {
		execCommand = shellCmdToBinaryCmd(command.ShellCmd)
	}

	commandTimeout := defaultTimeout
	if command.TimeoutSeconds != 0 {
		commandTimeout = time.Duration(command.TimeoutSeconds) * time.Second
	}

	context, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	exitCode, stdout, err := commandRunner(context, execCommand.Name, execCommand.Args, true)
	if exitCode == -1 && err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", command, err)
	}

	return newResolvedInstance(eval.NewInstance(
		eval.VarMap{
			compliance.CommandFieldExitCode: exitCode,
			compliance.CommandFieldStdout:   string(stdout),
		}, nil),
		execCommand.Name, "command",
	), nil
}
