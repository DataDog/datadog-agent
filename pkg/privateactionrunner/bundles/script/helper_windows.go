// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"context"
	"os/exec"
	"strings"
)

func NewShellScriptCommand(ctx context.Context, scriptFile string, args []string) *exec.Cmd {
	// -ExecutionPolicy Bypass to allow script execution
	// -File to run the script file with arguments
	psArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptFile}
	psArgs = append(psArgs, args...)
	return exec.CommandContext(ctx, "powershell.exe", psArgs...)
}

// On Windows, the command is executed via PowerShell without sudo wrapper.
func NewPredefinedScriptCommand(ctx context.Context, command []string, envVarNames []string) *exec.Cmd {
	if len(command) == 0 {
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", "Write-Output 'No command specified'")
	}

	firstArg := command[0]

	if isPowerShellScript(firstArg) {
		psArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File"}
		psArgs = append(psArgs, command...)
		return exec.CommandContext(ctx, "powershell.exe", psArgs...)
	}

	// Quote each argument to preserve spaces and special characters
	commandStr := buildPowerShellCommand(command)
	psArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", commandStr}
	return exec.CommandContext(ctx, "powershell.exe", psArgs...)
}

func buildPowerShellCommand(command []string) string {
	if len(command) == 0 {
		return ""
	}

	parts := make([]string, len(command))
	parts[0] = command[0] // Command/cmdlet itself

	for i := 1; i < len(command); i++ {
		// Escape single quotes by doubling them, then wrap in single quotes
		parts[i] = "'" + strings.ReplaceAll(command[i], "'", "''") + "'"
	}

	return strings.Join(parts, " ")
}

func isPowerShellScript(command string) bool {
	lowerCmd := strings.ToLower(command)
	return strings.HasSuffix(lowerCmd, ".ps1")
}
