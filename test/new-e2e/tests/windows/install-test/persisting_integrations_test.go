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
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpgrade tests upgrading the agent from WINDOWS_AGENT_VERSION to UPGRADE_TEST_VERSION
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
		)
		s.Require().NoError(err, "Agent should be %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	productVersionPre, err := windowsAgent.GetDatadogProductVersion(vm)
	s.Require().NoError(err, "should get product version")

	// enable persisting integrations
	s.enableIntegrationsPersistence(vm)

	// install third party integration
	err = s.installThirdPartyIntegration(vm, "datadog-ping==1.0.2")
	s.Require().NoError(err, "should install third party integration")

	// upgrade to test agent
	if !s.Run(fmt.Sprintf("upgrade to %s", s.upgradeAgentPackge.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.upgradeAgentPackge),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade.log")),
			windowsAgent.WithValidAPIKey(),
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
	s.CheckIntegrationInstall(vm, "datadog-ping==1.0.2")

	s.uninstallAgentAndRunUninstallTests(t)

}

// enableIntegrationsPersistence enables the persisting integrations feature
func (s *testPersistingIntegrationsSuite) enableIntegrationsPersistence(vm *components.RemoteHost) {
	// create the .install_python_third_party_deps file
	installPythonThirdPartyDepsFile := filepath.Join("C:\\ProgramData\\Datadog\\protected\\", ".install_python_third_party_deps")
	_, err := vm.Execute(fmt.Sprintf("echo %s > %s", "1", installPythonThirdPartyDepsFile))
	s.Require().NoError(err, "should create .install_python_third_party_deps file")
}

// install third party integration
func (s *testPersistingIntegrationsSuite) installThirdPartyIntegration(vm *components.RemoteHost, integration string) error {
	installPath, err := windowsAgent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err, "should get install path from registry")

	cmd := fmt.Sprintf(`& "%s\bin\agent.exe" integration install -t %s`, installPath, integration)
	out, err := vm.Execute(cmd)

	if err != nil && out != "" {
		s.T().Logf("Error installing integration %s:\n%s", integration, out)
	}

	return err
}

func (s *testPersistingIntegrationsSuite) CheckIntegrationInstall(vm *components.RemoteHost, integration string) {
	// check that the third party integration is still installed
	installPath, err := windowsAgent.GetInstallPathFromRegistry(vm)
	s.Require().NoError(err, "should get install path from registry")

	cmd := fmt.Sprintf(`& "%s\bin\agent.exe" integration freeze`, installPath)
	out, err := vm.Execute(cmd)
	s.Require().NoError(err, "should list integrations")

	// we use strings.Contains to limit output on failure
	assert.True(s.T(), strings.Contains(out, integration), "third party integration should be installed")
}
