// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !local

package com_datadoghq_script

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

var (
	ScriptUserName = "dd-scriptuser"
)

// shellQuote returns s wrapped in POSIX single quotes with embedded
// single quotes properly escaped.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellQuoteArgs joins args into a single shell-safe command string.
func shellQuoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " ")
}

func buildEnv(allowedEnvVars []string) []string {
	env := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	for _, name := range allowedEnvVars {
		if val, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+val)
		}
	}
	return env
}

func NewShellScriptCommand(ctx context.Context, scriptFile string, args []string) *exec.Cmd {
	shellCmd := shellQuoteArgs(append([]string{"sh", scriptFile}, args...))
	cmd := exec.CommandContext(ctx, "su", ScriptUserName, "-s", "/bin/sh", "-c", shellCmd)
	cmd.Env = buildEnv(nil)
	return cmd
}

func NewPredefinedScriptCommand(ctx context.Context, command []string, envVarNames []string) *exec.Cmd {
	shellCmd := shellQuoteArgs(command)
	cmd := exec.CommandContext(ctx, "su", ScriptUserName, "-s", "/bin/sh", "-c", shellCmd)
	cmd.Env = buildEnv(envVarNames)
	return cmd
}
