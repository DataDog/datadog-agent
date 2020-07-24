// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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

	commandFieldExitCode = "command.exitCode"
	commandFieldStdout   = "command.stdout"
)

var commandReportedFields = []string{
	commandFieldExitCode,
}

func checkCommand(_ env.Env, ruleID string, res compliance.Resource, expr *eval.IterableExpression) (*report, error) {
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

	context, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	exitCode, stdout, err := commandRunner(context, execCommand.Name, execCommand.Args, true)
	if exitCode == -1 && err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", command, err)
	}

	instance := &eval.Instance{
		Vars: eval.VarMap{
			commandFieldExitCode: exitCode,
			commandFieldStdout:   string(stdout),
		},
	}

	passed, err := expr.Evaluate(instance)
	if err != nil {
		return nil, err
	}

	return instanceToReport(instance, passed, commandReportedFields), nil
}
