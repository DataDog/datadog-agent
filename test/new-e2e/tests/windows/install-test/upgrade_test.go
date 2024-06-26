// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"path/filepath"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	servicetest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"

	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpgrade(t *testing.T) {
	s := &testUpgradeSuite{}
	run(t, s)
}

type testUpgradeSuite struct {
	baseAgentMSISuite
}

func (s *testUpgradeSuite) TestUpgrade() {
	vm := s.Env().RemoteHost

	// install previous version
	_ = s.installLastStable(vm)

	// simulate upgrading from a version that didn't have the runtime-security.d directory
	// to ensure upgrade places new config files.
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
	s.Require().NoError(err)
	err = vm.RemoveAll(filepath.Join(configRoot, "runtime-security.d"))
	s.Require().NoError(err)

	// upgrade to the new version
	if !s.Run(fmt.Sprintf("upgrade to %s", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "upgrade.log")),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// run tests
	t := s.newTester(vm)
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	s.uninstallAgentAndRunUninstallTests(t)
}

func TestUpgradeRollback(t *testing.T) {
	s := &testUpgradeRollbackSuite{}
	run(t, s)
}

type testUpgradeRollbackSuite struct {
	baseAgentMSISuite
}

// TC-INS-002
func (s *testUpgradeRollbackSuite) TestUpgradeRollback() {
	vm := s.Env().RemoteHost

	// install previous version
	previousTester := s.installLastStable(vm)

	// upgrade to the new version, but intentionally fail
	if !s.Run(fmt.Sprintf("upgrade to %s with rollback", s.AgentPackage.AgentVersion()), func() {
		_, err := windowsAgent.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "upgrade.log")),
		)
		s.Require().Error(err, "should fail to install agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err := windowsCommon.StartService(vm, "DatadogAgent")
	s.Require().NoError(err, "agent service should start after rollback")

	// the previous version should be functional
	if !previousTester.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	s.uninstallAgentAndRunUninstallTests(previousTester)
}

func TestUpgradeChangeUser(t *testing.T) {
	s := &testUpgradeChangeUserSuite{}
	run(t, s)
}

type testUpgradeChangeUserSuite struct {
	baseAgentMSISuite
}

func (s *testUpgradeChangeUserSuite) TestUpgradeChangeUser() {
	host := s.Env().RemoteHost

	oldUserName := windowsAgent.DefaultAgentUserName
	newUserName := "newagentuser"
	s.Require().NotEqual(oldUserName, newUserName, "new user name should be different from the default")

	// install previous version with defaults
	_ = s.installLastStable(host)

	// upgrade to the new version
	if !s.Run(fmt.Sprintf("upgrade to %s", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(host,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "upgrade.log")),
			windowsAgent.WithAgentUser(newUserName),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// run tests, checking for new user
	t := s.newTester(host,
		WithExpectedAgentUserName(newUserName),
	)
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	// old user shouldn't be deleted, so Identity should still exist
	oldUserIdentity, err := windowsCommon.GetIdentityForUser(host, oldUserName)
	s.Require().NoError(err)

	s.Run("removes file and registry permissions for old user", func() {
		installPath, err := windowsAgent.GetInstallPathFromRegistry(host)
		s.Require().NoError(err)
		configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
		s.Require().NoError(err)
		paths := []string{
			configRoot,
			filepath.Join(installPath, "embedded3"),
			windowsAgent.RegistryKeyPath,
		}

		// oldUserIdentity should not have permissions on the paths
		for _, path := range paths {
			out, err := windowsCommon.GetSecurityInfoForPath(host, path)
			s.Require().NoError(err)
			s.Assert().Empty(windowsCommon.FilterRulesForIdentity(out.Access, oldUserIdentity),
				"%s should not have permissions on %s", oldUserIdentity, path)
		}
	})
	s.Run("removes service permissions for old user", func() {
		// services should not have permissions for the old user
		serviceConfigs, err := windowsCommon.GetServiceConfigMap(host, servicetest.ExpectedInstalledServices())
		s.Require().NoError(err)
		for _, serviceName := range servicetest.ExpectedInstalledServices() {
			conf := serviceConfigs[serviceName]
			if windowsCommon.IsKernelModeServiceType(conf.ServiceType) {
				// we don't modify kernel mode services
				continue
			}
			out, err := windowsCommon.GetServiceSecurityInfo(host, serviceName)
			s.Require().NoError(err)
			s.Assert().Empty(windowsCommon.FilterRulesForIdentity(out.Access, oldUserIdentity),
				"%s should not have permissions on %s", oldUserIdentity, serviceName)
		}
	})

	s.uninstallAgentAndRunUninstallTests(t)
}

// TestUpgrade5to6to7 tests the upgrade path of Agent 5 -> Agent 6 -> Agent 7
func TestUpgrade5to6to7(t *testing.T) {
	var err error
	s := &testUpgrade5to6to7Suite{}
	// last stable agent 5
	s.agent5Package = &windowsAgent.Package{
		Version: "5.32.8-1",
	}
	s.agent5Package.URL, err = windowsAgent.GetStableMSIURL(s.agent5Package.Version, "x86_64")
	require.NoError(t, err)
	// last stable agent 6
	s.agent6Package = &windowsAgent.Package{
		Version: "6.53.0-1",
	}
	s.agent6Package.URL, err = windowsAgent.GetStableMSIURL(s.agent6Package.Version, "x86_64")
	require.NoError(t, err)
	// agent 7 version comes from env as in other tests
	run(t, s)
}

type testUpgrade5to6to7Suite struct {
	baseAgentMSISuite
	agent5Package *windowsAgent.Package
	agent6Package *windowsAgent.Package
}

func (s *testUpgrade5to6to7Suite) TestUpgrade5to6to7() {
	host := s.Env().RemoteHost
	var logFile string

	// agent 5
	if !s.installAgent5() {
		s.T().FailNow()
	}

	// agent 6
	logFile = filepath.Join(s.OutputDir, "upgrade-agent6.log")
	_ = s.installAndTestPreviousAgentVersion(host, s.agent6Package,
		windowsAgent.WithInstallLogFile(logFile),
	)
	if !s.migrateAgent5Config() {
		s.T().FailNow()
	}

	// agent 7
	logFile = filepath.Join(s.OutputDir, "upgrade-agent7.log")
	_ = s.installAgentPackage(host, s.AgentPackage,
		windowsAgent.WithInstallLogFile(logFile),
	)
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(host), s.AgentPackage.AgentVersion())

	// TODO: The import command creates datadog.yaml so it has Owner:Administrator Group:None,
	//       and the permissions tests expect Owner:SYSTEM Group:System
}

func (s *testUpgrade5to6to7Suite) installAgent5() bool {
	host := s.Env().RemoteHost
	agentPackage := s.agent5Package

	logFile := filepath.Join(s.OutputDir, "install-agent5.log")
	_, err := s.InstallAgent(host,
		windowsAgent.WithPackage(agentPackage),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithInstallLogFile(logFile),
	)
	s.Require().NoError(err, "should install agent 5")

	// get agent info
	installPath := windowsAgent.DefaultInstallPath
	cmd := fmt.Sprintf(`& "%s\embedded\python.exe" "%s\agent\agent.py" info`, installPath, installPath)
	out, err := host.Execute(cmd)
	s.Require().NoError(err, "should get agent info")
	s.T().Logf("Agent 5 info:\n%s", out)

	// basic checks to ensure agent is functioning
	s.Assert().Contains(out, agentPackage.AgentVersion(), "info should have agent 5 version")
	s.Assert().Contains(out, host.Address, "info should have IP address")
	confPath := `C:\ProgramData\Datadog\datadog.conf`
	exists, err := host.FileExists(confPath)
	s.Require().NoError(err, "should check if datadog.conf exists")
	s.Assert().True(exists, "datadog.conf should exist")

	return !s.T().Failed()
}

func (s *testUpgrade5to6to7Suite) migrateAgent5Config() bool {
	host := s.Env().RemoteHost

	installPath := windowsAgent.DefaultInstallPath
	configRoot := windowsAgent.DefaultConfigRoot
	cmd := fmt.Sprintf(`& "%s\bin\agent.exe" import "%s" "%s" --force`, installPath, configRoot, configRoot)
	out, err := host.Execute(cmd)
	s.Require().NoError(err, "should migrate agent 5 config")
	s.T().Logf("Migrate agent 5 config:\n%s", out)
	s.Assert().Contains(out, "Success: imported the contents of", "migrate agent 5 config should succeed")

	return !s.T().Failed()
}
