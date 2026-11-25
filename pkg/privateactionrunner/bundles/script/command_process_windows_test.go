// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func processTestCommand(t *testing.T, ctx context.Context, exe string, args []string) *exec.Cmd {
	t.Helper()
	return newPowershellCommand(ctx, &evaluatedPowershellScript{Script: processTestScript(exe, args)}, nil)
}

func processTestScript(exe string, args []string) string {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	// Launch with inherited standard handles. PowerShell's native-command
	// pipeline otherwise introduces its own output capture and waiting behavior.
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = syscall.EscapeArg(arg)
	}
	command := "$p = New-Object System.Diagnostics.ProcessStartInfo; " +
		"$p.FileName = " + quote(exe) + "; " +
		"$p.Arguments = " + quote(strings.Join(escaped, " ")) + "; " +
		"$p.UseShellExecute = $false; " +
		"$child = [System.Diagnostics.Process]::Start($p); $child.WaitForExit(); exit $child.ExitCode"

	return command
}

func processTestConfig(exe string, args []string) ScriptBundleConfig {
	return ScriptBundleConfig{SchemaId: schemaIdV1, RunPredefinedPowershellScript: map[string]RunPredefinedPowershellScriptConfig{
		"test": {Script: processTestScript(exe, args)},
	}}
}

func timeoutTestConfig() ScriptBundleConfig {
	return ScriptBundleConfig{SchemaId: schemaIdV1, RunPredefinedPowershellScript: map[string]RunPredefinedPowershellScriptConfig{
		"test": {Script: "Start-Sleep -Seconds 60"},
	}}
}
