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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func buildEnv(allowedEnvVars []string, credentialEnvVars []privateconnection.PrivateCredentialsToken) []string {
	env := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	credentialNames := make(map[string]struct{}, len(credentialEnvVars))
	for _, variable := range credentialEnvVars {
		credentialNames[variable.Name] = struct{}{}
	}
	for _, name := range allowedEnvVars {
		if _, suppliedByCredentials := credentialNames[name]; suppliedByCredentials {
			continue
		}
		if val, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+val)
		}
	}
	for _, variable := range credentialEnvVars {
		env = append(env, variable.Name+"="+variable.Value)
	}
	return env
}

func NewShellScriptCommand(ctx context.Context, scriptFile string, args []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", scriptFile}, args...)...)
	cmd.Env = buildEnv(nil, nil)
	return cmd, nil
}

func NewPredefinedScriptCommand(ctx context.Context, command []string, envVarNames []string, credentialEnvVars []privateconnection.PrivateCredentialsToken) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = buildEnv(envVarNames, credentialEnvVars)
	return cmd, nil
}
