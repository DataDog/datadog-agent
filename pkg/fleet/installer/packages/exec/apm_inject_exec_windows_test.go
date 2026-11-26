// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package exec

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

func TestAPMInjectExecLogPath(t *testing.T) {
	originalRootTmpDir := paths.RootTmpDir
	t.Cleanup(func() { paths.RootTmpDir = originalRootTmpDir })

	t.Run("defaults to the installer tmp directory", func(t *testing.T) {
		paths.RootTmpDir = t.TempDir()
		cmd := NewAPMInjectExec("ddinjector-installer.exe").newAPMInjectExecCmd(context.Background(), "install")
		assert.Equal(t, []string{"ddinjector-installer.exe", "install", "--log-path", paths.RootTmpDir}, cmd.Args)
	})

	t.Run("keeps the installer default when the tmp directory does not exist", func(t *testing.T) {
		paths.RootTmpDir = filepath.Join(t.TempDir(), "missing")
		cmd := NewAPMInjectExec("ddinjector-installer.exe").newAPMInjectExecCmd(context.Background(), "install")
		assert.Equal(t, []string{"ddinjector-installer.exe", "install"}, cmd.Args)
	})

	t.Run("WithLogPath overrides the default", func(t *testing.T) {
		paths.RootTmpDir = t.TempDir()
		logPath := t.TempDir()
		cmd := NewAPMInjectExec("ddinjector-installer.exe").WithLogPath(logPath).newAPMInjectExecCmd(context.Background(), "install")
		assert.Equal(t, []string{"ddinjector-installer.exe", "install", "--log-path", logPath}, cmd.Args)
	})
}
