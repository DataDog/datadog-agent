// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	commandutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/command"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var reportedFields = []string{
	compliance.CommandFieldExitCode,
}

func resolve(ctx context.Context, _ env.Env, ruleID string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
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
		execCommand = commandutils.ShellCmdToBinaryCmd(command.ShellCmd)
	}

	commandTimeout := compliance.DefaultTimeout
	if command.TimeoutSeconds != 0 {
		commandTimeout = time.Duration(command.TimeoutSeconds) * time.Second
	}

	context, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	exitCode, stdout, err := commandutils.Runner(context, execCommand.Name, execCommand.Args, true)
	if exitCode == -1 && err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", command, err)
	}

	stdoutStr := string(stdout)
	instance := eval.NewInstance(
		eval.VarMap{
			compliance.CommandFieldExitCode: exitCode,
			compliance.CommandFieldStdout:   stdoutStr,
		},
		nil,
		eval.RegoInputMap{
			"exitCode": exitCode,
			"stdout":   stdoutStr,
		},
	)

	return resources.NewResolvedInstance(instance, execCommand.Name, "command"), nil
}

func init() {
	resources.RegisterHandler("command", resolve, reportedFields)
}
