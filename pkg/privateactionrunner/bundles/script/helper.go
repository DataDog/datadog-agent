// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"os"
	"os/exec"
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
	cmd := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", scriptFile}, args...)...)
	cmd.Env = buildEnv(nil)
	return cmd, nil
}

func NewPredefinedScriptCommand(ctx context.Context, command []string, envVarNames []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = buildEnv(envVarNames)
	return cmd, nil
}
