// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	pkgversion "github.com/DataDog/datadog-agent/pkg/version"
)

// TestInstallerBinFromAgentExe verifies that installerBinFromAgentExe resolves
// the correct platform-specific installer path and returns a clear error when
// the binary does not exist.
func TestInstallerBinFromAgentExe(t *testing.T) {
	// Build a temporary directory tree that mirrors the real on-disk layout:
	//   Linux/macOS: <root>/bin/agent/agent + <root>/embedded/bin/installer
	//   Windows:     <root>/bin/agent/agent.exe + <root>/bin/datadog-installer.exe
	root := t.TempDir()

	agentDir := filepath.Join(root, "bin", "agent")
	require.NoError(t, os.MkdirAll(agentDir, 0755))

	var installerPath string
	if runtime.GOOS == "windows" {
		winRoot := defaultpaths.GetInstallPath()
		binDir := filepath.Join(winRoot, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0755))
		installerPath = filepath.Join(binDir, "datadog-installer.exe")
	} else {
		embeddedBinDir := filepath.Join(root, "embedded", "bin")
		require.NoError(t, os.MkdirAll(embeddedBinDir, 0755))
		installerPath = filepath.Join(embeddedBinDir, "installer")
	}
	require.NoError(t, os.WriteFile(installerPath, []byte("fake"), 0755))

	agentExe := filepath.Join(agentDir, "agent")

	t.Run("found", func(t *testing.T) {
		got, err := installerBinFromAgentExe(agentExe)
		require.NoError(t, err)
		assert.Equal(t, installerPath, got)
	})

	t.Run("not found", func(t *testing.T) {
		require.NoError(t, os.Remove(installerPath))
		_, err := installerBinFromAgentExe(agentExe)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "installer binary not found")
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
