// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !local && !windows

package com_datadoghq_script

import (
	"context"
	"os"
	"os/exec"
)

var (
	ScriptUserName = "dd-scriptuser"
)

func buildEnv(allowedEnvVars []string) []string {
	env := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	for _, name := range allowedEnvVars {
		if val, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+val)
		}
	}
	return env
}

func NewShellScriptCommand(ctx context.Context, scriptFile string, args []string) (*exec.Cmd, error) {
	shellCmd, err := shellQuoteArgs(append([]string{"sh", scriptFile}, args...))
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "su", ScriptUserName, "-s", "/bin/sh", "-c", shellCmd)
	cmd.Env = buildEnv(nil)
	return cmd, nil
}

func NewPredefinedScriptCommand(ctx context.Context, command []string, envVarNames []string) (*exec.Cmd, error) {
	shellCmd, err := shellQuoteArgs(command)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "su", ScriptUserName, "-s", "/bin/sh", "-c", shellCmd)
	cmd.Env = buildEnv(envVarNames)
	return cmd, nil
}
