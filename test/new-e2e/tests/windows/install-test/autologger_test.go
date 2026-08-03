// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"io/fs"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// TestInstallWithAutologger verifies that a plain install always configures the ETW
// AutoLogger in a disabled state (Start=0), grants the agent user the ACL needed to
// toggle it at runtime, and cleans it up on uninstall.
func TestInstallWithAutologger(t *testing.T) {
	s := &testInstallWithAutologgerSuite{}
	Run(t, s)
}

type testInstallWithAutologgerSuite struct {
	baseAgentMSISuite
}

func (s *testInstallWithAutologgerSuite) TestInstallWithAutologger() {
	vm := s.Env().RemoteHost

	_ = s.installAgentPackage(vm, s.AgentPackage)

	RequireAgentRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm))

	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	s.Run("autologger registry keys exist", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		assert.True(s.T(), exists, "autologger session key should always be configured on install")
	})

	s.Run("autologger Start value is 0", func() {
		val, err := windows.GetRegistryValue(vm, autologgerPath, "Start")
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "0", val, "autologger should be configured but disabled by default (Start=0)")
	})

	s.Run("autologger registry permissions", func() {
		hostname, err := windows.GetHostname(vm)
		require.NoError(s.T(), err)
		ddAgentUserIdentity, err := windows.GetIdentityForUser(vm,
			windows.MakeDownLevelLogonName(
				windows.NameToNetBIOSName(hostname),
				windowsAgent.DefaultAgentUserName,
			),
		)
		require.NoError(s.T(), err)

		if !windows.IsIdentityLocalSystem(ddAgentUserIdentity) {
			out, err := windows.GetSecurityInfoForPath(vm, autologgerPath)
			require.NoError(s.T(), err)
			agentUserRule := windows.NewExplicitAccessRule(
				ddAgentUserIdentity,
				windows.KEY_SET_VALUE|windows.KEY_QUERY_VALUE,
				windows.AccessControlTypeAllow,
			)
			windows.AssertContainsEqualable(s.T(), out.Access, agentUserRule,
				"%s should have access rule for %s", autologgerPath, ddAgentUserIdentity)
		}
	})

	s.Run("autologger providers are filtered (EnableLevel=4 + MatchAnyKeyword)", func() {
		// Provider GUID -> MatchAnyKeyword mask. Must mirror ProviderKeywords in
		// tools/windows/DatadogAgentInstaller/CustomActions/AutoLoggerCustomAction.cs.
		// Pins the exact filter so an accidental change to the installer is caught here.
		expected := map[string]uint64{
			"{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}": 0x10,               // Kernel-Process
			"{A68CA8B7-004F-D7B6-A698-07E2DE0F1F5D}": 0x80,               // Kernel-General
			"{DBE9B383-7CF3-4331-91CC-A3CB16A3B538}": 0x200000030000,     // Winlogon
			"{89B1E9F0-5AFF-44A6-9B44-0A07A7CE5845}": 0x6001000000000000, // User Profiles Service
			"{AEA1B4FA-97D1-45F2-A64C-4D69FFFD92C9}": 0x4000000000000000, // GroupPolicy
			"{30336ED4-E327-447C-9DE0-51B652C86108}": 0x4010000,          // Shell-Core
		}
		for guid, kw := range expected {
			providerPath := autologgerPath + `\` + guid

			lvl, err := windows.GetRegistryValue(vm, providerPath, "EnableLevel")
			require.NoError(s.T(), err)
			assert.Equal(s.T(), "4", lvl,
				"provider %s should be enabled at Informational level (4), not Verbose", guid)

			mask, err := windows.GetRegistryValue(vm, providerPath, "MatchAnyKeyword")
			require.NoError(s.T(), err)
			assert.Equal(s.T(), strconv.FormatUint(kw, 10), mask,
				"provider %s MatchAnyKeyword should match the analyzer's consumed keywords", guid)
		}
	})

	s.Run("uninstall removes autologger", func() {
		configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
		s.Require().NoError(err)

		s.Require().True(s.uninstallAgent())

		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		assert.NoError(s.T(), err)
		assert.False(s.T(), exists, "autologger session key should be removed after uninstall")

		logonDurationDir := configRoot + `\logonduration`
		_, err = vm.Lstat(logonDurationDir)
		assert.ErrorIs(s.T(), err, fs.ErrNotExist, "uninstall should remove logonduration directory")
	})
}

// TestAutologgerEnabledWhenLogonDurationEnabled verifies the runtime toggle: the
// autologger is created disabled (Start=0) on install, and the agent flips it to
// enabled (Start=1) once logon_duration.enabled is set in datadog.yaml and the agent
// restarts. This is the path that makes "enable after install" work without a reinstall.
func TestAutologgerEnabledWhenLogonDurationEnabled(t *testing.T) {
	s := &testAutologgerEnabledWhenLogonDurationEnabledSuite{}
	Run(t, s)
}

type testAutologgerEnabledWhenLogonDurationEnabledSuite struct {
	baseAgentMSISuite
}

func (s *testAutologgerEnabledWhenLogonDurationEnabledSuite) TestAutologgerEnabledWhenLogonDurationEnabled() {
	vm := s.Env().RemoteHost

	_ = s.installAgentPackage(vm, s.AgentPackage)

	RequireAgentRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm))

	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	s.Run("autologger is disabled by default (Start=0)", func() {
		val, err := windows.GetRegistryValue(vm, autologgerPath, "Start")
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "0", val,
			"autologger should be disabled while logon_duration.enabled is false")
	})

	s.Run("enabling logon_duration flips autologger to Start=1", func() {
		configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
		require.NoError(s.T(), err)
		configPath := filepath.Join(configRoot, "datadog.yaml")

		cfg, err := s.readYamlConfig(vm, configPath)
		require.NoError(s.T(), err)
		cfg["logon_duration"] = map[string]any{"enabled": true}
		require.NoError(s.T(), s.writeYamlConfig(vm, configPath, cfg))

		require.NoError(s.T(), windows.RestartService(vm, "datadogagent"))

		// The component enables the autologger asynchronously on startup, so poll.
		assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
			val, err := windows.GetRegistryValue(vm, autologgerPath, "Start")
			assert.NoError(c, err)
			assert.Equal(c, "1", val,
				"autologger should be enabled (Start=1) after logon_duration.enabled is set")
		}, 60*time.Second, 2*time.Second)
	})

	s.Run("reinstall with DD_INSTALL_ONLY=1 preserves Start=1", func() {
		// Simulate an install-only upgrade (e.g. via SCCM) where the agent service
		// is not restarted. The MSI custom action must not reset Start back to 0
		// because the agent never gets a chance to re-apply the runtime toggle.
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithInstallOnly("1"),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "reinstall.log")),
		)
		require.NoError(s.T(), err)

		val, err := windows.GetRegistryValue(vm, autologgerPath, "Start")
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "1", val,
			"autologger Start should remain 1 after DD_INSTALL_ONLY=1 reinstall (agent not restarted)")
	})
}

// TestInstallRollbackRemovesAutologger verifies that when a clean install fails
// (triggering rollback), the autologger keys created during the install are removed.
func TestInstallRollbackRemovesAutologger(t *testing.T) {
	s := &testInstallRollbackRemovesAutologgerSuite{}
	Run(t, s)
}

type testInstallRollbackRemovesAutologgerSuite struct {
	baseAgentMSISuite
}

func (s *testInstallRollbackRemovesAutologgerSuite) TestInstallRollbackRemovesAutologger() {
	vm := s.Env().RemoteHost
	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	s.Run("no autologger exists before install", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		require.False(s.T(), exists)
	})

	s.Run("install fails and triggers rollback", func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
		)
		require.Error(s.T(), err, "should fail to install agent")
	})

	s.Run("rollback removes the freshly-created autologger", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		assert.False(s.T(), exists,
			"autologger created during install should be removed by rollback")
	})
}

// TestInstallRollbackPreservesAutologger verifies that when an autologger already
// exists and an install fails (triggering rollback), the pre-existing autologger is
// restored. The action snapshots the existing keys before overwriting them, so rollback
// reinstates the original values.
func TestInstallRollbackPreservesAutologger(t *testing.T) {
	s := &testInstallRollbackPreservesAutologgerSuite{}
	Run(t, s)
}

type testInstallRollbackPreservesAutologgerSuite struct {
	baseAgentMSISuite
}

func (s *testInstallRollbackPreservesAutologgerSuite) TestInstallRollbackPreservesAutologger() {
	vm := s.Env().RemoteHost
	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	// Create pre-existing autologger registry keys to simulate a previous install.
	createPreExistingAutologgerKeys(s.T(), vm, autologgerPath)

	s.Run("pre-existing autologger exists before failed install", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		require.True(s.T(), exists)
	})

	s.Run("install fails and triggers rollback", func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
		)
		require.Error(s.T(), err, "should fail to install agent")
	})

	s.Run("rollback preserves pre-existing autologger", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		assert.True(s.T(), exists,
			"pre-existing autologger should still exist after rollback (snapshot was restored)")
	})

	s.Run("rollback preserves pre-existing autologger values", func() {
		val, err := windows.GetRegistryValue(vm, autologgerPath, "Guid")
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "{00000000-0000-0000-0000-000000000000}", val,
			"autologger Guid should be preserved after rollback")
	})
}

// createPreExistingAutologgerKeys creates a minimal autologger registry key tree
// to simulate a pre-existing autologger from a previous install. Uses a distinctive
// Guid value (all-zeros) and a distinctive Start value (42) to verify the install
// preserves the original state rather than recreating it from scratch.
func createPreExistingAutologgerKeys(t *testing.T, vm *components.RemoteHost, autologgerPath string) {
	t.Helper()
	err := windows.SetRegistryDWORDValue(vm, autologgerPath, "Start", 42)
	require.NoError(t, err, "should create pre-existing autologger Start value")
	err = windows.SetTypedRegistryValue(vm, autologgerPath, "Guid", "{00000000-0000-0000-0000-000000000000}", "String")
	require.NoError(t, err, "should create pre-existing autologger Guid value")
}
