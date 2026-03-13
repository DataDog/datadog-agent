// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

func TestInstallWithAutologger(t *testing.T) {
	s := &testInstallWithAutologgerSuite{}
	Run(t, s)
}

type testInstallWithAutologgerSuite struct {
	baseAgentMSISuite
}

func (s *testInstallWithAutologgerSuite) TestInstallWithAutologger() {
	vm := s.Env().RemoteHost

	_ = s.installAgentPackage(vm, s.AgentPackage,
		windowsAgent.WithLogonDurationAutologger("true"),
	)

	RequireAgentRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm))

	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	s.Run("autologger registry keys exist", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		assert.True(s.T(), exists, "autologger session key should exist after install with DD_LOGON_DURATION_AUTOLOGGER=true")
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

	s.Run("uninstall removes autologger", func() {
		s.Require().True(s.uninstallAgent())

		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		assert.NoError(s.T(), err)
		assert.False(s.T(), exists, "autologger session key should be removed after uninstall")
	})
}

// TestInstallWithAutologgerRollback verifies that when DD_LOGON_DURATION_AUTOLOGGER=true is set
// and the install fails (triggering rollback), a pre-existing autologger is restored.
func TestInstallWithAutologgerRollback(t *testing.T) {
	s := &testInstallWithAutologgerRollbackSuite{}
	Run(t, s)
}

type testInstallWithAutologgerRollbackSuite struct {
	baseAgentMSISuite
}

func (s *testInstallWithAutologgerRollbackSuite) TestInstallWithAutologgerRollback() {
	vm := s.Env().RemoteHost
	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	// Create pre-existing autologger registry keys to simulate a previous install
	createPreExistingAutologgerKeys(s.T(), vm, autologgerPath)

	s.Run("pre-existing autologger exists before failed install", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		require.True(s.T(), exists)
	})

	s.Run("install with autologger fails and triggers rollback", func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithLogonDurationAutologger("true"),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
		)
		require.Error(s.T(), err, "should fail to install agent")
	})

	s.Run("rollback restores pre-existing autologger", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		assert.True(s.T(), exists,
			"autologger should still exist after rollback (was pre-existing)")
	})

	s.Run("rollback restores pre-existing autologger values", func() {
		val, err := windows.GetRegistryValue(vm, autologgerPath, "Start")
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "42", val,
			"autologger Start value should be restored to pre-existing value")
	})
}

// TestInstallWithoutAutologgerRollback verifies that when DD_LOGON_DURATION_AUTOLOGGER is NOT set
// and the install fails (triggering rollback), a pre-existing autologger is preserved.
func TestInstallWithoutAutologgerRollback(t *testing.T) {
	s := &testInstallWithoutAutologgerRollbackSuite{}
	Run(t, s)
}

type testInstallWithoutAutologgerRollbackSuite struct {
	baseAgentMSISuite
}

func (s *testInstallWithoutAutologgerRollbackSuite) TestInstallWithoutAutologgerRollback() {
	vm := s.Env().RemoteHost
	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	// Create pre-existing autologger registry keys to simulate a previous install
	createPreExistingAutologgerKeys(s.T(), vm, autologgerPath)

	s.Run("pre-existing autologger exists before failed install", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		require.True(s.T(), exists)
	})

	s.Run("install without autologger fails and triggers rollback", func() {
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
			"autologger should still exist after rollback (flag was not set, rollback is no-op)")
	})

	s.Run("rollback preserves pre-existing autologger values", func() {
		val, err := windows.GetRegistryValue(vm, autologgerPath, "Start")
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "42", val,
			"autologger Start value should be preserved after rollback")
	})
}

// createPreExistingAutologgerKeys creates a minimal autologger registry key tree
// to simulate a pre-existing autologger from a previous install. Uses a distinctive
// Start value (42) to verify rollback restores the original state rather than
// recreating it from scratch.
func createPreExistingAutologgerKeys(t *testing.T, vm *components.RemoteHost, autologgerPath string) {
	t.Helper()
	err := windows.SetRegistryDWORDValue(vm, autologgerPath, "Start", 42)
	require.NoError(t, err, "should create pre-existing autologger Start value")
	err = windows.SetTypedRegistryValue(vm, autologgerPath, "Guid", "{00000000-0000-0000-0000-000000000000}", "String")
	require.NoError(t, err, "should create pre-existing autologger Guid value")
}

func TestInstallWithoutAutologger(t *testing.T) {
	s := &testInstallWithoutAutologgerSuite{}
	Run(t, s)
}

type testInstallWithoutAutologgerSuite struct {
	baseAgentMSISuite
}

func (s *testInstallWithoutAutologgerSuite) TestInstallWithoutAutologger() {
	vm := s.Env().RemoteHost

	_ = s.installAgentPackage(vm, s.AgentPackage)

	RequireAgentRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm))

	autologgerPath := windowsAgent.AutologgerRegistryKeyPath

	s.Run("autologger registry keys should not exist", func() {
		exists, err := windows.RegistryKeyExists(vm, autologgerPath)
		require.NoError(s.T(), err)
		assert.False(s.T(), exists,
			"autologger session key should not exist without DD_LOGON_DURATION_AUTOLOGGER=true")
	})

	s.Run("logonduration directory should not exist", func() {
		configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
		require.NoError(s.T(), err)
		logonDurationDir := filepath.Join(configRoot, "logonduration")
		exists, err := vm.FileExists(logonDurationDir)
		if assert.NoError(s.T(), err) {
			assert.False(s.T(), exists,
				"logonduration directory should not exist without DD_LOGON_DURATION_AUTOLOGGER=true")
		}
	})

	s.cleanupOnSuccessInDevMode()
}
