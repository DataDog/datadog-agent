// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"fmt"
	"os/user"
	"strings"
)

func (h *TestConnectionHandler) validateScriptUser() (string, []string) {
	var errors []string
	var info strings.Builder

	scriptUserInfo, err := user.Lookup(ScriptUserName)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Script user '%s' not found: %v", ScriptUserName, err))
	} else {
		info.WriteString(fmt.Sprintf("Script user '%s' found (UID: %s, GID: %s)\n",
			scriptUserInfo.Username, scriptUserInfo.Uid, scriptUserInfo.Gid))
	}

	// Check if the current user can run command as the script user via su
	cmd, err := NewPredefinedScriptCommand(context.Background(), []string{"echo", "test"}, nil)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to build test command: %v", err))
		return info.String(), errors
	}
	_, err = cmd.CombinedOutput()
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to check if the current user can use the script user: %v", err))
	} else {
		info.WriteString("Current user can use the script user.\n")
	}

	return info.String(), errors
}

