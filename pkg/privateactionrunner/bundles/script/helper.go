// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !local

package com_datadoghq_script

import (
	"context"
	"os/exec"
)

func NewPredefinedScriptCommand(ctx context.Context, command []string) *exec.Cmd {
	sudoArgs := append([]string{"-u", "scriptuser"}, command...)
	return exec.CommandContext(ctx, "sudo", sudoArgs...)
}
