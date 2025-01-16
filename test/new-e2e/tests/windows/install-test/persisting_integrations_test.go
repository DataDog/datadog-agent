// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	servicetest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPersistingIntegrations tests upgrading the agent from WINDOWS_AGENT_VERSION to UPGRADE_TEST_VERSION
func TestPersistingIntegrations(t *testing.T) {
	s := &testPersistingIntegrationsSuite{}
	upgradeAgentPackge, err := windowsAgent.GetUpgradeTestPackageFromEnv()
	require.NoError(t, err, "should get upgrade test package")
	s.upgradeAgentPackge = upgradeAgentPackge
	run(t, s)
}

type testPersistingIntegrationsSuite struct {
	baseAgentMSISuite
	upgradeAgentPackge *windowsAgent.Package
}

func (s *testPersistingIntegrationsSuite) TestPersistingIntegrations() {
	vm := s.Env().RemoteHost

	// install current version
	if !s.Run(fmt.Sprintf("install %s", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithIntegrationsPersistence("1"),
		)
		s.Require().NoError(err, "Agent should be %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	productVersionPre, err := windowsAgent.GetDatadogProductVersion(vm)
	s.Require().NoError(err, "should get product version")

	// install third party integration
	err = s.installThirdPartyIntegration(vm, "datadog-ping==1.0.2")
	s.Require().NoError(err, "should install third party integration")

	// install pip package
	err = s.installPipPackage(vm, "grpcio")
	s.Require().NoError(err, "should install pip package")

	// upgrade to test agent
	if !s.Run(fmt.Sprintf("upgrade to %s", s.upgradeAgentPackge.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.upgradeAgentPackge),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithIntegrationsPersistence("1"),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.upgradeAgentPackge.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// run tests
	testerOptions := []TesterOption{
		WithAgentPackage(s.upgradeAgentPackge),
	}
	t, err := NewTester(s, vm, testerOptions...)
	s.Require().NoError(err, "should create tester")
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	// Get Display Version
	productVersionPost, err := windowsAgent.GetDatadogProductVersion(vm)
	s.Require().NoError(err, "should get product version")

	// check that version is different post upgrade
	assert.NotEqual(s.T(), productVersionPre, productVersionPost, "product version should be different after upgrade")

	// check that the third party integration is still installed
	s.checkIntegrationInstall(vm, "datadog-ping==1.0.2")

	// check that the pip package is still installed
	s.checkPipPackageInstalled(vm, "grpcio")

	s.uninstallAgentAndRunUninstallTests(t)

}

// TestPersistingIntegrations tests upgrading the agent from WINDOWS_AGENT_VERSION to UPGRADE_TEST_VERSION
func TestIntegrationInstallFailure(t *testing.T) {
	s := &testIntegrationInstallFailure{}
	run(t, s)
}

type testIntegrationInstallFailure struct {
	baseAgentMSISuite
}

func (s *testIntegrationInstallFailure) TestIntegrationInstallFailure() {
	vm := s.Env().RemoteHost

	// install previous version
	previousAgentPackage := s.installAndTestLastStable(vm)

	// upgrade to the new version, but intentionally fail with our persistence flag
	if !s.Run(fmt.Sprintf("upgrade to %s with rollback", s.AgentPackage.AgentVersion()), func() {
		_, err := windowsAgent.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
			windowsAgent.WithIntegrationsPersistence("1"),
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
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), previousAgentPackage.AgentVersion())

	// Ensure services are still installed
	// NOTE: will need to update this if we add or remove services
	_, err = windowsCommon.GetServiceConfigMap(vm, servicetest.ExpectedInstalledServices())
	s.Assert().NoError(err, "services should still be installed")

	s.uninstallAgent()

}

// TestIntegrationFolderPermissions tests upgrading the agent from WINDOWS_AGENT_VERSION to UPGRADE_TEST_VERSION
// this tests the agent will not install if the folder permissions are incorrect
func TestIntegrationFolderPermissions(t *testing.T) {
	s := &testIntegrationFolderPermissions{}
	upgradeAgentPackge, err := windowsAgent.GetUpgradeTestPackageFromEnv()
	require.NoError(t, err, "should get upgrade test package")
	s.upgradeAgentPackge = upgradeAgentPackge
	run(t, s)
}

type testIntegrationFolderPermissions struct {
	baseAgentMSISuite
	upgradeAgentPackge *windowsAgent.Package
}

func (s *testIntegrationFolderPermissions) TestIntegrationFolderPermissions() {
	vm := s.Env().RemoteHost

	// install current version
	if !s.Run(fmt.Sprintf("install %s", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
			windowsAgent.WithValidAPIKey(),
			// disable integrations persistence
			windowsAgent.WithIntegrationsPersistence("0"),
		)
		s.Require().NoError(err, "Agent should be %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// create folder in protected location with the ddagentuser as the owner
	// run tests
	testerOptions := []TesterOption{
		WithAgentPackage(s.AgentPackage),
	}
	t, err := NewTester(s, vm, testerOptions...)
	s.Require().NoError(err, "should create tester")
	ddAgentUserIdentity, err := windowsCommon.GetIdentityForUser(t.host,
		windowsCommon.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
	)
	s.Require().NoError(err, "should get ddagentuser identity")

	// create folder owned to ddAgentUserIdenity
	folderPath := "C:\\ProgramData\\Datadog\\protected"
	err = vm.MkdirAll(folderPath)
	s.Require().NoError(err, "should create folder")

	// write file to folder
	filePath := filepath.Join(folderPath, ".diff_python_installed_packages.txt")
	_, err = vm.WriteFile(filePath, []byte(""))
	s.Require().NoError(err, "should write file to folder")

	// run powershell command to own file to ddAgentUserIdentity
	cmd := fmt.Sprintf(`$acl = Get-Acl "%s"; $acl.SetOwner([System.Security.Principal.NTAccount]::new("%s")); Set-Acl "%s" $acl`, filePath, ddAgentUserIdentity.GetName(), filePath)
	_, err = vm.Execute(cmd)
	s.Require().NoError(err, "should set owner to ddAgentUserIdentity")

	// upgrade to the new version, should fail due to folder permissions
	if !s.Run(fmt.Sprintf("Install %s with failure", s.upgradeAgentPackge.AgentVersion()), func() {
		_, err := windowsAgent.InstallAgent(vm,
			windowsAgent.WithPackage(s.upgradeAgentPackge),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
			windowsAgent.WithIntegrationsPersistence("1"),
		)
		s.Require().Error(err, "should fail to install agent %s", s.upgradeAgentPackge.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err = windowsCommon.StartService(vm, "DatadogAgent")
	s.Require().NoError(err, "agent service should start after rollback")

	// the previous version should be functional
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), s.AgentPackage.AgentVersion())

	// Ensure services are still installed
	// NOTE: will need to update this if we add or remove services
	_, err = windowsCommon.GetServiceConfigMap(vm, servicetest.ExpectedInstalledServices())
	s.Assert().NoError(err, "services should still be installed")

	s.uninstallAgent()

}

// install third party integration
func (s *testPersistingIntegrationsSuite) installThirdPartyIntegration(vm *components.RemoteHost, integration string) error {
	installPath, err := windowsAgent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err, "should get install path from registry")

	cmd := fmt.Sprintf(`& "%s\bin\agent.exe" integration install -t %s`, installPath, integration)
	_, err = vm.Execute(cmd)

	if err != nil {
		s.T().Logf("Error installing integration %s:\n%s", integration, err)
	}

	return err
}

// install pip package
func (s *testPersistingIntegrationsSuite) installPipPackage(vm *components.RemoteHost, packageToInstall string) error {
	installPath, err := windowsAgent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err, "should get install path from registry")

	cmd := fmt.Sprintf(`& "%s\embedded3\python.exe" -m pip install %s`, installPath, packageToInstall)
	_, err = vm.Execute(cmd)

	if err != nil {
		s.T().Logf("Error installing pip package %s:\n%s", packageToInstall, err)
	}

	return err
}

// check pip package is installed
func (s *testPersistingIntegrationsSuite) checkPipPackageInstalled(vm *components.RemoteHost, packageToCheck string) {
	installPath, err := windowsAgent.GetInstallPathFromRegistry(vm)
	s.Require().NoError(err, "should get install path from registry")

	cmd := fmt.Sprintf(`& "%s\embedded3\python.exe" -m pip show %s`, installPath, packageToCheck)
	out, err := vm.Execute(cmd)
	s.Require().NoError(err, "should show pip package")

	// check to make sure it is installed
	packageCheck := fmt.Sprintf("Name: %s", packageToCheck)
	assert.True(s.T(), strings.Contains(out, packageCheck), "pip package should be installed")
}

func (s *testPersistingIntegrationsSuite) checkIntegrationInstall(vm *components.RemoteHost, integration string) {
	// check that the third party integration is still installed
	installPath, err := windowsAgent.GetInstallPathFromRegistry(vm)
	s.Require().NoError(err, "should get install path from registry")

	cmd := fmt.Sprintf(`& "%s\bin\agent.exe" integration freeze`, installPath)
	out, err := vm.Execute(cmd)
	s.Require().NoError(err, "should list integrations")

	// we use strings.Contains to limit output on failure
	assert.True(s.T(), strings.Contains(out, integration), "third party integration should be installed")
}
