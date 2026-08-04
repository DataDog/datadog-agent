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
	"strings"
)

func buildEnv(allowedEnvVars []string, extraPaths ...string) []string {
	basePath := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	pathVal := basePath
	if len(extraPaths) > 0 {
		pathVal = strings.Join(extraPaths, ":") + ":" + basePath
	}
	env := []string{"PATH=" + pathVal}
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

// NewPredefinedScriptCommand builds a command for a predefined script.
// extraPaths are prepended to PATH so tool binaries extracted from catalog packages
// are found before system binaries. extraEnvVars are set unconditionally.
func NewPredefinedScriptCommand(ctx context.Context, command []string, envVarNames []string, extraEnvVars map[string]string, extraPaths []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = buildEnv(envVarNames, extraPaths...)
	for k, v := range extraEnvVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd, nil
}
