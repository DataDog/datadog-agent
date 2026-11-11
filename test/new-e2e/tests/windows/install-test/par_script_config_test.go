// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// TestPARScriptConfig verifies that a customer-edited Private Action Runner script config
// survives MSI upgrades and a failed-upgrade rollback.
//
// It guards against reintroducing the MSI-owned live config behind incident 61851, where
// upgrades replaced customer edits with the packaged example. Upgrades from affected versions
// (7.77.0 to 7.84.2) still lose the config by design and are not covered.
//
// The scenarios share one VM; each method removes the Agent and its config when it ends.
func TestPARScriptConfig(t *testing.T) {
	s := &testPARScriptConfigSuite{}
	var err error
	s.lastStablePackage, err = windowsAgent.GetLastStablePackageFromEnv()
	require.NoError(t, err, "should get last stable agent package from env")
	s.upgradeTestPackage, err = windowsAgent.GetUpgradeTestPackageFromEnv()
	require.NoError(t, err, "should get upgrade test agent package from env")
	Run(t, s)
}

type testPARScriptConfigSuite struct {
	baseAgentMSISuite
	lastStablePackage  *windowsAgent.Package
	upgradeTestPackage *windowsAgent.Package
}

// parScriptConfigCustom uses a script name that never appears in the packaged example.
const parScriptConfigCustom = `schemaId: script-credentials-v1
runPredefinedPowershellScript:
  e2eMsiUpgradePreserved:
    script: |
      Write-Output "preserved"
`

// TestUpgradeFromLastStable upgrades from the last stable MSI to this pipeline's MSI, so a
// regression in this pipeline's install actions (e.g. WriteConfig overwriting) is caught.
func (s *testPARScriptConfigSuite) TestUpgradeFromLastStable() {
	vm := s.Env().RemoteHost
	defer s.cleanupAgent()

	s.installAndTestPreviousAgentVersion(vm, s.lastStablePackage)
	s.customizePARScriptConfig(vm)

	s.installOrFail(vm, s.AgentPackage, "upgrade.log")
	s.assertPARScriptConfigPreserved(vm, "after upgrade")
}

// TestUpgradeToUpgradeTestPackage upgrades from this pipeline's MSI to its "-upgrade-test"
// variant. The pipeline MSI is the old product here, so if it owns the live config, removing
// it during the major upgrade deletes the customer edit.
func (s *testPARScriptConfigSuite) TestUpgradeToUpgradeTestPackage() {
	vm := s.Env().RemoteHost
	defer s.cleanupAgent()

	s.installOrFail(vm, s.AgentPackage, "install.log")
	s.customizePARScriptConfig(vm)

	s.installOrFail(vm, s.upgradeTestPackage, "upgrade.log")
	s.assertPARScriptConfigPreserved(vm, "after upgrade")
}

// TestFailedUpgradeRollback fails an upgrade to this pipeline's MSI and checks the rollback
// to the last stable keeps the customer edit.
func (s *testPARScriptConfigSuite) TestFailedUpgradeRollback() {
	vm := s.Env().RemoteHost
	defer s.cleanupAgent()

	s.installAndTestPreviousAgentVersion(vm, s.lastStablePackage)
	s.customizePARScriptConfig(vm)

	if !s.Run("upgrade with rollback", func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "upgrade-rollback.log")),
		)
		s.Require().Error(err, "upgrade to %s should fail", s.AgentPackage.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}
	s.assertPARScriptConfigPreserved(vm, "after upgrade rollback")
}

func (s *testPARScriptConfigSuite) installOrFail(vm *components.RemoteHost, pkg *windowsAgent.Package, logName string) {
	if !s.Run("install "+pkg.AgentVersion(), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(pkg),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), logName)),
		)
		s.Require().NoError(err, "should install agent %s", pkg.AgentVersion())
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}
}

func (s *testPARScriptConfigSuite) parScriptConfigPath(vm *components.RemoteHost) string {
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
	s.Require().NoError(err)
	return filepath.Join(configRoot, "private-action-runner", "powershell-script-config.yaml")
}

// customizePARScriptConfig replaces the installed PAR script config with a customer-style edit,
// after checking the install created it from the template. Stops the test when it fails, so a
// later preservation failure is not misattributed.
func (s *testPARScriptConfigSuite) customizePARScriptConfig(vm *components.RemoteHost) {
	if !s.Run("customize PAR script config", func() {
		path := s.parScriptConfigPath(vm)
		live, err := vm.ReadFile(path)
		s.Require().NoError(err, "install should create the live PAR script config")
		s.Require().NotEmpty(live)
		example, err := vm.ReadFile(path + ".example")
		s.Require().NoError(err, "install should ship the PAR script config template")
		s.Require().NotEmpty(example)

		_, err = vm.WriteFile(path, []byte(parScriptConfigCustom))
		s.Require().NoError(err)
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}
}

func (s *testPARScriptConfigSuite) assertPARScriptConfigPreserved(vm *components.RemoteHost, stage string) {
	s.Run("PAR script config preserved "+stage, func() {
		path := s.parScriptConfigPath(vm)
		live, err := vm.ReadFile(path)
		s.Require().NoError(err)
		s.Assert().Equal(strings.TrimSpace(parScriptConfigCustom), strings.TrimSpace(string(live)),
			"PAR script config was modified %s", stage)
		example, err := vm.ReadFile(path + ".example")
		s.Require().NoError(err, "PAR script config template should still ship %s", stage)
		s.Assert().NotEmpty(example)
	})
}
