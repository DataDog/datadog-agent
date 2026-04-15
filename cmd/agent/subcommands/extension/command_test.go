// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgversion "github.com/DataDog/datadog-agent/pkg/version"
)

// TestResolveInstallerBinPath verifies that resolveInstallerBinPath finds the
// installer binary relative to the agent executable, and returns a clear error
// when the installer binary does not exist.
func TestResolveInstallerBinPath(t *testing.T) {
	// Build a temporary directory tree that mirrors the real on-disk layout:
	//   <root>/bin/agent/agent   (fake agent exe)
	//   <root>/embedded/bin/installer[.exe]
	root := t.TempDir()

	agentDir := filepath.Join(root, "bin", "agent")
	require.NoError(t, os.MkdirAll(agentDir, 0755))

	embeddedBinDir := filepath.Join(root, "embedded", "bin")
	require.NoError(t, os.MkdirAll(embeddedBinDir, 0755))

	installerName := "installer"
	if runtime.GOOS == "windows" {
		installerName = "installer.exe"
	}
	installerPath := filepath.Join(embeddedBinDir, installerName)
	require.NoError(t, os.WriteFile(installerPath, []byte("fake"), 0755))

	t.Run("found", func(t *testing.T) {
		agentExe := filepath.Join(agentDir, "agent")
		got := filepath.Clean(filepath.Join(
			filepath.Dir(agentExe), "..", "..", "embedded", "bin", installerName,
		))
		assert.Equal(t, installerPath, got)
		_, err := os.Stat(got)
		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		require.NoError(t, os.Remove(installerPath))

		agentExe := filepath.Join(agentDir, "agent")
		candidate := filepath.Clean(filepath.Join(
			filepath.Dir(agentExe), "..", "..", "embedded", "bin", installerName,
		))
		_, err := os.Stat(candidate)
		assert.True(t, os.IsNotExist(err), "expected not-found error, got: %v", err)
	})
}

// TestResolvePackageURL verifies the three URL resolution modes.
func TestResolvePackageURL(t *testing.T) {
	// Ensure a consistent version for test assertions.
	orig := pkgversion.AgentVersionURLSafe
	pkgversion.AgentVersionURLSafe = "7.78.0"
	t.Cleanup(func() { pkgversion.AgentVersionURLSafe = orig })

	t.Run("explicit url takes precedence", func(t *testing.T) {
		explicit := "oci://custom.registry.io/agent-package:7.78.0-1"
		got, err := resolvePackageURL(explicit, "ignored.registry.io/some/path")
		require.NoError(t, err)
		assert.Equal(t, explicit, got)
	})

	t.Run("registry flag constructs url with current version", func(t *testing.T) {
		got, err := resolvePackageURL("", "us-central1-docker.pkg.dev/myproject/byoc")
		require.NoError(t, err)
		assert.Equal(t, "oci://us-central1-docker.pkg.dev/myproject/byoc/agent-package:7.78.0-1", got)
	})

	t.Run("registry flag strips leading oci:// if present", func(t *testing.T) {
		got, err := resolvePackageURL("", "oci://us-central1-docker.pkg.dev/myproject/byoc")
		require.NoError(t, err)
		assert.Equal(t, "oci://us-central1-docker.pkg.dev/myproject/byoc/agent-package:7.78.0-1", got)
	})

	t.Run("registry flag strips trailing slash", func(t *testing.T) {
		got, err := resolvePackageURL("", "us-central1-docker.pkg.dev/myproject/byoc/")
		require.NoError(t, err)
		assert.Equal(t, "oci://us-central1-docker.pkg.dev/myproject/byoc/agent-package:7.78.0-1", got)
	})

	t.Run("default datadog registry when no flags", func(t *testing.T) {
		t.Setenv("DD_SITE", "datadoghq.com")
		got, err := resolvePackageURL("", "")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(got, "oci://install.datadoghq.com/"), "expected datadoghq.com registry, got: %s", got)
		assert.Contains(t, got, "7.78.0-1")
	})

	t.Run("version already has -1 suffix", func(t *testing.T) {
		pkgversion.AgentVersionURLSafe = "7.78.0-1"
		got, err := resolvePackageURL("", "myregistry.io/byoc")
		require.NoError(t, err)
		// Should not double-append -1
		assert.Equal(t, "oci://myregistry.io/byoc/agent-package:7.78.0-1", got)
	})
}

// TestInstallCommandArgs verifies cobra argument validation for the install subcommand.
func TestInstallCommandArgs(t *testing.T) {
	cmd := installCommand()

	t.Run("no args fails", func(t *testing.T) {
		err := cmd.ValidateArgs([]string{})
		assert.Error(t, err)
	})

	t.Run("one extension passes", func(t *testing.T) {
		err := cmd.ValidateArgs([]string{"ddot"})
		assert.NoError(t, err)
	})

	t.Run("multiple extensions pass", func(t *testing.T) {
		err := cmd.ValidateArgs([]string{"ddot", "other"})
		assert.NoError(t, err)
	})
}

// TestRemoveCommandArgs verifies cobra argument validation for the remove subcommand.
func TestRemoveCommandArgs(t *testing.T) {
	cmd := removeCommand()

	t.Run("no args fails", func(t *testing.T) {
		err := cmd.ValidateArgs([]string{})
		assert.Error(t, err)
	})

	t.Run("one extension passes", func(t *testing.T) {
		err := cmd.ValidateArgs([]string{"ddot"})
		assert.NoError(t, err)
	})

	t.Run("multiple extensions pass", func(t *testing.T) {
		err := cmd.ValidateArgs([]string{"ddot", "other"})
		assert.NoError(t, err)
	})
}
