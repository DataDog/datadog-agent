// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"bytes"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
)

func (h *TestConnectionHandler) validateScriptUser() (string, []string) {
	var errors []string
	var info strings.Builder

	scriptUserInfo, err := user.Lookup(ScriptUserName)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Script user '%s' not found: %v. Run the provisioning script to create the account.", ScriptUserName, err))
		return info.String(), errors
	}
	info.WriteString(fmt.Sprintf("Script user '%s' found (SID: %s)\n",
		scriptUserInfo.Username, scriptUserInfo.Uid))

	token, err := logonScriptUser()
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to logon as script user '%s': %v", ScriptUserName, err))
		return info.String(), errors
	}
	defer token.Close()
	info.WriteString(fmt.Sprintf("Successfully obtained logon token for '%s'.\n", ScriptUserName))

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "whoami")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Token: token,
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Env = buildAllowedEnv(nil)

	err = cmd.Run()
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to run test command as script user: %v", err))
		return info.String(), errors
	}

	whoami := strings.TrimSpace(stdout.String())
	if !strings.Contains(strings.ToLower(whoami), strings.ToLower(ScriptUserName)) {
		errors = append(errors, fmt.Sprintf("Test command ran as '%s' instead of expected '%s'", whoami, ScriptUserName))
	} else {
		info.WriteString(fmt.Sprintf("Test command confirmed execution as '%s'.\n", whoami))
	}

	return info.String(), errors
}

