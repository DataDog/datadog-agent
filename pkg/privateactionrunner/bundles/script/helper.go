// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !local

package com_datadoghq_script

import (
	"context"
	"os/exec"
	"strings"
)

func NewShellScriptCommand(ctx context.Context, scriptFile string, args []string) *exec.Cmd {
	sudoArgs := []string{"-u", "scriptuser", "sh", scriptFile}
	sudoArgs = append(sudoArgs, args...)
	return exec.CommandContext(ctx, "sudo", sudoArgs...)
}

func NewPredefinedScriptCommand(ctx context.Context, command []string, envVarNames []string) *exec.Cmd {
	sudoArgs := []string{"-u", "scriptuser"}
	if len(envVarNames) > 0 {
		preserveEnvArg := "--preserve-env=" + strings.Join(envVarNames, ",")
		sudoArgs = append(sudoArgs, preserveEnvArg)
	}
	sudoArgs = append(sudoArgs, command...)

	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)
	return cmd
}
