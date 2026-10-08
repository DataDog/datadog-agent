// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestWindowsPackageHookCommands(t *testing.T) {
	systemDir, err := windows.GetSystemDirectory()
	require.NoError(t, err)
	root := filepath.Join(t.TempDir(), "package with spaces & !literal", "hooks")
	for _, extension := range []string{".exe", ".ps1", ".bat"} {
		t.Run(extension, func(t *testing.T) {
			path := filepath.Join(root, "postInstall"+extension)
			cmd, err := newPackageHookCommand(context.Background(), path)
			require.NoError(t, err)
			require.NotNil(t, cmd.Cancel)
			switch extension {
			case ".exe":
				assert.Equal(t, path, cmd.Path)
				assert.Equal(t, []string{path}, cmd.Args)
			case ".ps1":
				assert.Equal(t, filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe"), cmd.Path)
				assert.Equal(t, []string{cmd.Path, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path}, cmd.Args)
			case ".bat":
				assert.Equal(t, filepath.Join(systemDir, "cmd.exe"), cmd.Path)
				require.NotNil(t, cmd.SysProcAttr)
				assert.Equal(t, windows.EscapeArg(cmd.Path)+` /D /V:OFF /S /C ""`+path+`""`, cmd.SysProcAttr.CmdLine)
			}
		})
	}
	_, err = newPackageHookCommand(context.Background(), filepath.Join(root, "postInstall.cmd"))
	require.Error(t, err)
	_, err = newPackageHookCommand(context.Background(), `C:\package%PATH%\hooks\postInstall.bat`)
	require.ErrorContains(t, err, "must not contain %")
}
