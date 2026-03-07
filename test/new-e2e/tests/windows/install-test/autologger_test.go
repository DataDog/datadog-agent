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
