// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	servicetest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestUpgrade tests upgrading the agent from the stable version to the current version
func TestUpgrade(t *testing.T) {
	s := &testUpgradeSuite{}
	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	require.NoError(t, err, "should get last stable agent package from env")
	s.previousAgentPackge = previousAgentPackage
	Run(t, s)
}

type testUpgradeSuite struct {
	baseAgentMSISuite
	previousAgentPackge *windowsAgent.Package
}

func (s *testUpgradeSuite) TestUpgrade() {
	vm := s.Env().RemoteHost

	// install previous version
	s.installAndTestPreviousAgentVersion(vm, s.previousAgentPackge)

	// simulate upgrading from a version that didn't have the runtime-security.d directory
	// to ensure upgrade places new config files.
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
	s.Require().NoError(err)
	path := filepath.Join(configRoot, "runtime-security.d")
	if _, err = vm.Lstat(path); err == nil {
		err = vm.RemoveAll(path)
		s.Require().NoError(err)
	}

	// upgrade to the new version
	if !s.Run("upgrade to "+s.AgentPackage.AgentVersion(), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// dd-procmgr-service runs as LocalSystem, so it stays enabled even when the upgrade
	// does not provide DDAGENTUSER_PASSWORD. Local accounts already behaved this way
	// because the installer generates a password for the default ddagentuser.
	s.Run("process manager stays enabled for a local account", func() {
		config, err := windowsCommon.GetServiceConfig(vm, "dd-procmgr-service")
		s.Require().NoError(err)
		s.Assert().Equal(windowsCommon.SERVICE_DEMAND_START, config.StartType,
			"dd-procmgr-service must stay enabled for local accounts")
	})

	// run tests
	t := s.newTester(vm)
	if !t.TestInstallExpectations(s.T()) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	s.uninstallAgentAndRunUninstallTests(t)
}

// TestUpgradeFromLatest tests upgrading the agent from the current version to the upgrade-test version
func TestUpgradeFromLatest(t *testing.T) {
	s := &testUpgradeFromLatestSuite{}
	upgradeAgentPackge, err := windowsAgent.GetUpgradeTestPackageFromEnv()
	require.NoError(t, err, "should get last stable agent package from env")
	s.upgradeAgentPackge = upgradeAgentPackge
	Run(t, s)
}

type testUpgradeFromLatestSuite struct {
	baseAgentMSISuite
	upgradeAgentPackge *windowsAgent.Package
}

func (s *testUpgradeFromLatestSuite) TestUpgradeFromLatest() {
	vm := s.Env().RemoteHost

	// install current version
	if !s.Run("install "+s.AgentPackage.AgentVersion(), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
			windowsAgent.WithValidAPIKey(),
		)
		s.Require().NoError(err, "Agent should be %s", s.AgentPackage.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	productVersionPre, err := windowsAgent.GetDatadogProductVersion(vm)
	s.Require().NoError(err, "should get product version")

	// This unowned value survives service reconfiguration but not deletion/recreation.
	const markerPath = `HKLM:\SYSTEM\CurrentControlSet\Services\datadogagent`
	s.Require().NoError(windowsCommon.SetRegistryDWORDValue(vm, markerPath, "E2ERecreationMarker", 1))

	// upgrade to test agent
	if !s.Run("upgrade to "+s.upgradeAgentPackge.AgentVersion(), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.upgradeAgentPackge),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
			windowsAgent.WithValidAPIKey(),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.upgradeAgentPackge.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	marker := vm.MustExecute(`(Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Services\datadogagent').PSObject.Properties.Name -contains 'E2ERecreationMarker'`)
	s.Require().Equal("False", strings.TrimSpace(marker), "upgrade must recreate services when credentials are stored")

	// run tests
	testerOptions := []TesterOption{
		WithAgentPackage(s.upgradeAgentPackge),
	}
	t, err := NewTester(s, vm, testerOptions...)
	s.Require().NoError(err, "should create tester")
	if !t.TestInstallExpectations(s.T()) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// Get Display Version
	productVersionPost, err := windowsAgent.GetDatadogProductVersion(vm)
	s.Require().NoError(err, "should get product version")

	// check that version is different post upgrade
	assert.NotEqual(s.T(), productVersionPre, productVersionPost, "product version should be different after upgrade")

	s.uninstallAgentAndRunUninstallTests(t)

}

// Keep each scenario in its own suite test so the existing BeforeTest/AfterTest
// hooks collect the deletion audit before the Security log can roll over.
func (s *testUpgradeFromLatestSuite) TestServiceDeletionRollbackLocalPasswordChange() {
	s.testServiceDeletionRollback(serviceDeletionRollbackCase{restoreAvailable: true})
}

func (s *testUpgradeFromLatestSuite) TestServiceDeletionRollbackLocalMissingPassword() {
	s.testServiceDeletionRollback(serviceDeletionRollbackCase{missingPassword: true, restoreAvailable: true})
}

func (s *testUpgradeFromLatestSuite) TestServiceDeletionRollbackLocalSystem() {
	s.testServiceDeletionRollback(serviceDeletionRollbackCase{account: "LocalSystem", restoreAvailable: true})
}

func (s *testUpgradeFromLatestSuite) TestServiceDeletionRollbackSwitchToLocalSystem() {
	s.testServiceDeletionRollback(serviceDeletionRollbackCase{incomingAccount: "LocalSystem"})
}

type serviceDeletionRollbackCase struct {
	account          string
	incomingAccount  string
	missingPassword  bool
	restoreAvailable bool
}

// Both MSIs contain the conditional deletion logic. Released source packages
// retain their old DeleteServices condition and cannot exercise this rollback.
func (s *testUpgradeFromLatestSuite) testServiceDeletionRollback(tc serviceDeletionRollbackCase) {
	s.T().Helper()
	vm := s.Env().RemoteHost
	_, err := s.InstallAgent(vm,
		windowsAgent.WithPackage(s.AgentPackage),
		windowsAgent.WithAgentUser(tc.account),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
	)
	s.Require().NoError(err)
	product, err := windowsAgent.GetDatadogAgentProductCode(vm)
	s.Require().NoError(err)
	originalConfig, err := windowsCommon.GetServiceConfigMap(vm, servicetest.ExpectedInstalledServices())
	s.Require().NoError(err)
	const markerPath = `HKLM:\SYSTEM\CurrentControlSet\Services\datadogagent`
	s.Require().NoError(windowsCommon.SetRegistryDWORDValue(vm, markerPath, "E2ERecreationMarker", 1))
	if tc.missingPassword {
		// Keep the account's real password intact; remove only the installer's secret.
		s.removeStoredAgentPassword()
	}
	logPath := filepath.Join(s.SessionOutputDir(), "upgrade-rollback.log")
	_, err = s.InstallAgent(vm,
		windowsAgent.WithAgentUserPassword("E2e-Changed-Password-472!"),
		windowsAgent.WithAgentUser(tc.incomingAccount),
		windowsAgent.WithPackage(s.upgradeAgentPackge),
		windowsAgent.WithWixFailWhenDeferred(),
		windowsAgent.WithInstallLogFile(logPath),
	)
	s.Require().Error(err)
	log, err := os.ReadFile(logPath)
	s.Require().NoError(err)
	log, err = windowsCommon.ConvertUTF16ToUTF8(log)
	s.Require().NoError(err)
	if tc.missingPassword {
		s.Require().Contains(string(log), "Preserving services:")
		marker := vm.MustExecute(`(Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Services\datadogagent').E2ERecreationMarker`)
		s.Require().Equal("1", strings.TrimSpace(marker), "rollback must retain the pre-existing service")
	} else {
		s.Require().Contains(string(log), "Allowing service deletion during upgrade removal", "must exercise conditional deletion")
	}
	restoredProduct, err := windowsAgent.GetDatadogAgentProductCode(vm)
	s.Require().NoError(err)
	s.Require().Equal(product, restoredProduct)
	restoredConfig, err := windowsCommon.GetServiceConfigMap(vm, servicetest.ExpectedInstalledServices())
	s.Require().NoError(err)
	// Running state is checked by starting the Agent below; it is not service configuration.
	for _, config := range originalConfig {
		config.Status = 0
	}
	for _, config := range restoredConfig {
		config.Status = 0
	}
	s.Require().Equal(originalConfig, restoredConfig, "rollback must restore service configuration")
	if tc.restoreAvailable {
		s.Require().NoError(windowsCommon.StartService(vm, "datadogagent"))
		RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), s.AgentPackage.AgentVersion())
	} else {
		s.Require().Contains(string(log), "current credentials unavailable for outgoing account; repair may be required")
	}
	s.uninstallAgent()
}

// Remove only the installer's LSA password, without changing the user's password
// or the credentials retained by SCM. This models an upgrade without a saved secret.
func (s *testUpgradeFromLatestSuite) removeStoredAgentPassword() {
	s.T().Helper()
	_, err := s.Env().RemoteHost.Execute(`
if (-not ('E2eLsaSecret' -as [type])) {
Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
public static class E2eLsaSecret {
    [StructLayout(LayoutKind.Sequential)]
    struct Attributes {
        public uint Length;
        public IntPtr RootDirectory, ObjectName;
        public uint Flags;
        public IntPtr SecurityDescriptor, SecurityQualityOfService;
    }
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    struct UnicodeString {
        public ushort Length, MaximumLength;
        [MarshalAs(UnmanagedType.LPWStr)] public string Buffer;
    }
    [DllImport("advapi32.dll")] static extern uint LsaOpenPolicy(IntPtr name, ref Attributes attrs, uint access, out IntPtr handle);
    [DllImport("advapi32.dll")] static extern uint LsaStorePrivateData(IntPtr handle, ref UnicodeString key, IntPtr data);
    [DllImport("advapi32.dll")] static extern uint LsaNtStatusToWinError(uint status);
    [DllImport("advapi32.dll")] static extern uint LsaClose(IntPtr handle);
    static void Check(uint status) {
        if (status != 0) throw new Win32Exception((int)LsaNtStatusToWinError(status));
    }
    public static void RemovePassword() {
        var attrs = new Attributes { Length = (uint)Marshal.SizeOf(typeof(Attributes)) };
        IntPtr handle;
        Check(LsaOpenPolicy(IntPtr.Zero, ref attrs, 0x20, out handle));
        try {
            const string name = "L$datadog_ddagentuser_password";
            var key = new UnicodeString { Buffer = name, Length = (ushort)(name.Length * 2), MaximumLength = (ushort)((name.Length + 1) * 2) };
            Check(LsaStorePrivateData(handle, ref key, IntPtr.Zero));
        } finally { LsaClose(handle); }
    }
}
'@
}
[E2eLsaSecret]::RemovePassword()
`)
	s.Require().NoError(err)
}

func TestUpgradeRollback(t *testing.T) {
	s := &testUpgradeRollbackSuite{}
	Run(t, s)
}

type testUpgradeRollbackSuite struct {
	baseAgentMSISuite
}

// TC-INS-002
func (s *testUpgradeRollbackSuite) TestUpgradeRollback() {
	vm := s.Env().RemoteHost

	// install previous version
	previousAgentPackage := s.installAndTestLastStable(vm)

	// upgrade to the new version, but intentionally fail
	if !s.Run(fmt.Sprintf("upgrade to %s with rollback", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
		)
		s.Require().Error(err, "should fail to install agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err := windowsCommon.StartService(vm, "DatadogAgent")
	s.Require().NoError(err, "agent service should start after rollback")

	// the previous version should be functional
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), previousAgentPackage.AgentVersion())

	// Ensure services are still installed
	// NOTE: will need to update this if we add or remove services
	_, err = windowsCommon.GetServiceConfigMap(vm, servicetest.ExpectedInstalledServices())
	s.Assert().NoError(err, "services should still be installed")

	s.uninstallAgent()
}

// TestUpgradeRollbackWithoutCWS tests that when upgrading the agent from X.51 to CURRENT_AGENT_VERSION
// rolls back, that the ddprocmon service is not installed.
func TestUpgradeRollbackWithoutCWS(t *testing.T) {
	s := &testUpgradeRollbackWithoutCWSSuite{}
	Run(t, s)
}

type testUpgradeRollbackWithoutCWSSuite struct {
	baseAgentMSISuite
	previousAgentPackage *windowsAgent.Package
}

func (s *testUpgradeRollbackWithoutCWSSuite) SetupSuite() {
	if setupSuite, ok := any(&s.baseAgentMSISuite).(suite.SetupAllSuite); ok {
		setupSuite.SetupSuite()
	}

	// CWS was GA in X.52, so start by installing X.51
	// match X to the major version from CURRENT_AGENT_VERSION
	var err error
	majorVersion := strings.Split(s.AgentPackage.Version, ".")[0]
	s.previousAgentPackage = &windowsAgent.Package{
		Version: majorVersion + ".51.0-1",
		Arch:    "x86_64",
	}
	s.previousAgentPackage.URL, err = windowsAgent.GetStableMSIURL(s.previousAgentPackage.Version, s.previousAgentPackage.Arch, "")
	s.Require().NoError(err, "should get stable agent package URL")
}

func (s *testUpgradeRollbackWithoutCWSSuite) TestUpgradeRollbackWithoutCWS() {
	vm := s.Env().RemoteHost

	// install previous version
	s.installAndTestPreviousAgentVersion(vm, s.previousAgentPackage)

	// upgrade to the new version, but intentionally fail
	if !s.Run(fmt.Sprintf("upgrade to %s with rollback", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
		)
		s.Require().Error(err, "should fail to install agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err := windowsCommon.StartService(vm, "DatadogAgent")
	s.Require().NoError(err, "agent service should start after rollback")

	// the previous version should be functional
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), s.previousAgentPackage.AgentVersion())

	// Ensure CWS services are not installed, but other services are
	cwsServices := []string{
		"ddprocmon",
		"datadog-security-agent",
	}
	// NOTE: will need to update this if we add or remove services
	expectedServices := servicetest.ExpectedInstalledServices()
	expectedServices = slices.DeleteFunc(expectedServices, func(s string) bool {
		return slices.Contains(cwsServices, s)
	})
	_, err = windowsCommon.GetServiceConfigMap(vm, expectedServices)
	s.Assert().NoError(err, "services should still be installed")
	for _, service := range cwsServices {
		_, err = windowsCommon.GetServiceConfig(vm, service)
		s.Assert().Error(err, "service %s should not be installed", service)
	}

	s.uninstallAgent()
}

func TestUpgradeChangeUser(t *testing.T) {
	s := &testUpgradeChangeUserSuite{}
	Run(t, s)
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
	s.installAndTestLastStable(host)

	// upgrade to the new version
	if !s.Run("upgrade to "+s.AgentPackage.AgentVersion(), func() {
		_, err := s.InstallAgent(host,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
			windowsAgent.WithAgentUser(newUserName),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// run tests, checking for new user
	t := s.newTester(host,
		WithExpectedAgentUserName(newUserName),
	)
	if !t.TestInstallExpectations(s.T()) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
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

// TestUpgradeFromV5 tests upgrading from Agent 5 to CURRENT_AGENT_VERSION
func TestUpgradeFromV5(t *testing.T) {
	var err error
	s := &testUpgradeFromV5Suite{}
	// last stable agent 5
	s.agent5Package = &windowsAgent.Package{
		Version: "5.32.8-1",
	}
	s.agent5Package.URL, err = windowsAgent.GetStableMSIURL(s.agent5Package.Version, "x86_64", "")
	require.NoError(t, err)
	Run(t, s)
}

type testUpgradeFromV5Suite struct {
	baseAgentMSISuite
	agent5Package *windowsAgent.Package
}

func (s *testUpgradeFromV5Suite) TestUpgrade5() {
	host := s.Env().RemoteHost

	// agent 5
	s.installAgent5()

	// upgrade to the new version
	if !s.Run("upgrade to "+s.AgentPackage.AgentVersion(), func() {
		_, err := s.InstallAgent(host,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// migrate config and verify agent is running
	s.migrateAgent5Config()
	err := windowsCommon.RestartService(host, "DatadogAgent")
	s.Require().NoError(err, "should restart agent service")
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(host), s.AgentPackage.AgentVersion())

	// TODO: The import command creates datadog.yaml so it has Owner:Administrator Group:None,
	//       and the permissions tests expect Owner:SYSTEM Group:System
	s.cleanupOnSuccessInDevMode()
}

func (s *testUpgradeFromV5Suite) installAgent5() {
	host := s.Env().RemoteHost
	agentPackage := s.agent5Package

	logFile := filepath.Join(s.SessionOutputDir(), "install-agent5.log")
	_, err := s.InstallAgent(host,
		windowsAgent.WithPackage(agentPackage),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithInstallLogFile(logFile),
	)
	s.Require().NoError(err, "should install agent 5")

	// get agent info
	// in loop because the agent may not be ready immediately after install/start
	installPath := windowsAgent.DefaultInstallPath
	s.Assert().EventuallyWithT(func(t *assert.CollectT) {
		cmd := fmt.Sprintf(`& "%s\embedded\python.exe" "%s\agent\agent.py" info`, installPath, installPath)
		out, err := host.Execute(cmd)
		require.NoError(t, err, "should get agent info")
		s.T().Logf("Agent 5 info:\n%s", out)
		assert.Contains(t, out, agentPackage.AgentVersion(), "info should have agent 5 version")
	}, 5*time.Minute, 5*time.Second, "should get agent 5 info")

	confPath := `C:\ProgramData\Datadog\datadog.conf`
	exists, err := host.FileExists(confPath)
	s.Require().NoError(err, "should check if datadog.conf exists")
	s.Assert().True(exists, "datadog.conf should exist")

	if s.T().Failed() {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}
}

func (s *testUpgradeFromV5Suite) migrateAgent5Config() {
	host := s.Env().RemoteHost

	installPath := windowsAgent.DefaultInstallPath
	configRoot := windowsAgent.DefaultConfigRoot
	cmd := fmt.Sprintf(`& "%s\bin\agent.exe" import "%s" "%s" --force`, installPath, configRoot, configRoot)
	out, err := host.Execute(cmd)
	s.Require().NoError(err, "should migrate agent 5 config")
	s.T().Logf("Migrate agent 5 config:\n%s", out)
	s.Require().Contains(out, "Success: imported the contents of", "migrate agent 5 config should succeed")
}

// TestUpgradeFromV6 tests upgrading from Agent 6 to CURRENT_AGENT_VERSION
func TestUpgradeFromV6(t *testing.T) {
	var err error
	s := &testUpgradeSuite{}
	s.previousAgentPackge = &windowsAgent.Package{
		Version: "6.53.0-1",
		Arch:    "x86_64",
	}
	s.previousAgentPackge.URL, err = windowsAgent.GetStableMSIURL(s.previousAgentPackge.Version, s.previousAgentPackge.Arch, "")
	require.NoError(t, err)
	Run(t, s)
}
