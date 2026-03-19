// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddottests implements E2E tests for the DDOT agent extension on Windows.
package ddottests

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testDDOTExtensionInstallScript struct {
	installerwindows.BaseSuite
}

// TestDDOTExtensionViaInstallScript verifies that the DDOT extension is installed
// when DD_OTELCOLLECTOR_ENABLED=true is passed to the install script, and that
// purge removes it.
func TestDDOTExtensionViaInstallScript(t *testing.T) {
	e2e.Run(t, &testDDOTExtensionInstallScript{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

func (s *testDDOTExtensionInstallScript) AfterTest(_suiteName, _testName string) {
	s.Installer().Purge()
}

func (s *testDDOTExtensionInstallScript) TestInstallAndPurgeDDOTExtension() {
	// Act: install the Agent via the install script with DDOT enabled
	output, err := s.InstallScript().Run(
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_OTELCOLLECTOR_ENABLED": "true",
		}),
	)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	// Assert: DDOT extension files exist under the agent package
	ddotExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", "ddot")
	s.Require().Host(s.Env().RemoteHost).DirExists(ddotExtDir, "ddot extension directory should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(ddotExtDir, "embedded", "bin", "otel-agent.exe"),
		"otel-agent.exe should be present in the ddot extension",
	)
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	// Assert: ddagentuser has explicit FullControl on otel-config.yaml
	otelConfigPath := filepath.Join(windowsagent.DefaultConfigRoot, "otel-config.yaml")
	requireDDAgentUserExplicitFullControl(s.T(), s.Env().RemoteHost, otelConfigPath)

	// Act: purge all packages
	_, err = s.Installer().Purge()
	s.Require().NoError(err)

	// Assert: extension directory and service are gone
	s.Require().Host(s.Env().RemoteHost).NoDirExists(ddotExtDir, "ddot extension should be removed after purge")
	s.Require().Host(s.Env().RemoteHost).HasNoService("datadog-otel-agent")

	// Assert: explicit ddagentuser ACE removed from otel-config.yaml
	requireNoDDAgentUserExplicitAccess(s.T(), s.Env().RemoteHost, otelConfigPath)
}

// requireDDAgentUserExplicitFullControl asserts that ddagentuser has an explicit
// (non-inherited) Allow FullControl ACE on the given path.
func requireDDAgentUserExplicitFullControl(t *testing.T, host *components.RemoteHost, path string) {
	t.Helper()
	ddAgentUser, err := windowscommon.GetIdentityForUser(host, windowsagent.DefaultAgentUserName)
	require.NoError(t, err, "should get ddagentuser identity")

	security, err := windowscommon.GetSecurityInfoForPath(host, path)
	require.NoError(t, err, "should get security info for %s", path)

	expected := windowscommon.NewExplicitAccessRule(ddAgentUser, windowscommon.FileFullControl, windowscommon.AccessControlTypeAllow)
	require.True(t,
		slices.ContainsFunc(security.Access, func(r windowscommon.AccessRule) bool { return r.Equal(expected) }),
		"ddagentuser should have explicit FullControl on %s", path)
	require.False(t, security.AreAccessRulesProtected,
		"DACL on %s should not be protected (should inherit from parent)", path)
}

// requireNoDDAgentUserExplicitAccess asserts that ddagentuser has no explicit
// (non-inherited) FullControl ACE on the given path. If the file no longer
// exists the assertion passes.
func requireNoDDAgentUserExplicitAccess(t *testing.T, host *components.RemoteHost, path string) {
	t.Helper()

	ddAgentUser, err := windowscommon.GetIdentityForUser(host, windowsagent.DefaultAgentUserName)
	require.NoError(t, err, "should get ddagentuser identity")

	security, err := windowscommon.GetSecurityInfoForPath(host, path)
	require.NoError(t, err, "should get security info for %s", path)

	expected := windowscommon.NewExplicitAccessRule(ddAgentUser, windowscommon.FileFullControl, windowscommon.AccessControlTypeAllow)
	require.False(t,
		slices.ContainsFunc(security.Access, func(r windowscommon.AccessRule) bool { return r.Equal(expected) }),
		"ddagentuser should not have explicit FullControl ACE on %s after removal", path)
	require.False(t, security.AreAccessRulesProtected,
		"DACL on %s should not be protected (should inherit from parent)", path)
}
