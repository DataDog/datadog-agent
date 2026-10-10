// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestConfigureCommandWindowsPreservesProcessAttributes(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	configureCommand(cmd)

	assert.True(t, cmd.SysProcAttr.HideWindow)
	assert.NotZero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP)
}

func TestNewCommandWindowsWrapsPowerShellEntrypoint(t *testing.T) {
	setRequiredWindowsEnvironment(t)
	session := newWindowsTestSession(t)
	entrypoint := filepath.Join(t.TempDir(), "run.ps1")
	pkg := &Package{
		Command: []string{entrypoint, "fixed argument"},
		Manifest: &Manifest{
			ParameterEnvMapping: map[string]string{"message": "PAR_MESSAGE"},
		},
	}

	cmd, err := NewCommand(context.Background(), pkg, session, map[string]interface{}{"message": "hello"})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), cmd.Path)
	assert.Equal(t, []string{
		cmd.Path,
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		entrypoint,
		"fixed argument",
	}, cmd.Args)
	assert.Equal(t, session.WorkDirectory, cmd.Dir)
}

func TestNewCommandWindowsRejectsUnsupportedEntrypoint(t *testing.T) {
	setRequiredWindowsEnvironment(t)
	session := newWindowsTestSession(t)
	pkg := &Package{Command: []string{`C:\package\run.sh`}, Manifest: &Manifest{}}

	_, err := NewCommand(context.Background(), pkg, session, nil)

	require.ErrorContains(t, err, "unsupported Windows extension")
}

func newWindowsTestSession(t *testing.T) *Session {
	t.Helper()
	root := t.TempDir()
	session := &Session{
		RootDirectory: root,
		WorkDirectory: filepath.Join(root, workDirectoryName),
		HomeDirectory: filepath.Join(root, homeDirectoryName),
		TempDirectory: filepath.Join(root, tempDirectoryName),
	}
	for _, directory := range []string{session.WorkDirectory, session.HomeDirectory, session.TempDirectory} {
		require.NoError(t, os.Mkdir(directory, sessionDirectoryMode))
	}
	return session
}

func setRequiredWindowsEnvironment(t *testing.T) {
	t.Helper()
	if os.Getenv("SYSTEMROOT") == "" {
		t.Setenv("SYSTEMROOT", `C:\Windows`)
	}
	if os.Getenv("WINDIR") == "" {
		t.Setenv("WINDIR", os.Getenv("SYSTEMROOT"))
	}
	if os.Getenv("COMSPEC") == "" {
		t.Setenv("COMSPEC", filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "cmd.exe"))
	}
	if os.Getenv("PATHEXT") == "" {
		t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	}
}
