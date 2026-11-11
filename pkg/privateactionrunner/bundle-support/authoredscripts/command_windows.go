// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const powerShellExecutable = "powershell.exe"

func platformCommand(ctx context.Context, command []string) (*exec.Cmd, error) {
	entrypoint := command[0]
	extension := strings.ToLower(filepath.Ext(entrypoint))
	switch extension {
	case ".ps1":
		systemRoot, err := windowsSystemRoot()
		if err != nil {
			return nil, err
		}
		powershell := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", powerShellExecutable)
		arguments := []string{
			powershell,
			"-NoLogo",
			"-NoProfile",
			"-NonInteractive",
			"-ExecutionPolicy",
			"Bypass",
			"-File",
			entrypoint,
		}
		command = append(arguments, command[1:]...)
	case ".exe", ".com":
		// Run native executables directly.
	default:
		return nil, fmt.Errorf("authored-script command %q has unsupported Windows extension %q; expected .ps1, .exe, or .com", entrypoint, extension)
	}
	return exec.CommandContext(ctx, command[0], command[1:]...), nil
}

func executionDeniedError(err error) error {
	return fmt.Errorf("authored-script execution was denied by the host; check file permissions, AppLocker, and Windows Defender Application Control policies: %w", err)
}
