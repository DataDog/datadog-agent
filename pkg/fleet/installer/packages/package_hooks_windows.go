// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func newPackageHookCommand(ctx context.Context, path string) (*exec.Cmd, error) {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, fmt.Errorf("could not locate Windows hook interpreters: %w", err)
	}
	var cmd *exec.Cmd
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe":
		cmd = exec.CommandContext(ctx, path)
	case ".ps1":
		cmd = exec.CommandContext(ctx, filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe"),
			"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path)
	case ".bat":
		// Percent expansion happens even within cmd.exe quotes. Fail closed
		// rather than letting a package path be interpreted as environment input.
		if strings.Contains(path, "%") {
			return nil, fmt.Errorf("Windows batch hook paths must not contain %%: %s", path)
		}
		cmd = exec.CommandContext(ctx, filepath.Join(systemDir, "cmd.exe"))
		// cmd.exe does not use CommandLineToArgvW quoting. /S /C strips the
		// outer pair; the inner pair protects spaces and shell metacharacters.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CmdLine: windows.EscapeArg(cmd.Path) + ` /D /V:OFF /S /C ""` + path + `""`,
		}
	default:
		return nil, fmt.Errorf("unsupported Windows package hook extension: %s", filepath.Ext(path))
	}
	cmd.Cancel = func() error {
		// Kill the tree while the parent still exists. Do not resolve taskkill
		// through a package-controlled PATH, and bound the cleanup itself.
		killCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		kill := exec.CommandContext(killCtx, filepath.Join(systemDir, "taskkill.exe"),
			"/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	return cmd, nil
}
