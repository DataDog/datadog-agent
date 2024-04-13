// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"bytes"
	"os/exec"
)

func setSELinuxPermissions() error {
	// Check if SELinux is enabled
	cmd := exec.Command("getenforce")
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	if err != nil {
		// SELinux is not installed, nothing to do
		return nil
	}
	if outb.String() != "Enforcing\n" {
		// SELinux is not enforcing, nothing to do
		return nil
	}

	err = executeCommand(seLinuxSetPermissionsCommand)
	if err != nil {
		return err
	}

	err = executeCommand(seLinuxRestoreContextCommand)
	if err != nil {
		return err
	}
	return nil
}
