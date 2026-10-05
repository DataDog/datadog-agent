// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

const (
	// eudmExtensionName is the fleet installer extension name (End User Device Monitoring).
	eudmExtensionName = "eudm"
	// aiUsageBinaryName is the AI Usage native host binary shipped in the eudm extension layer.
	aiUsageBinaryName = "ai-usage-agent-native-host.exe"
	// aiUsageConfigDir is the ProgramData config directory where the generated config lives.
	aiUsageConfigDir = `C:\ProgramData\Datadog`
	// aiUsageConfigName is the generated runtime config.
	aiUsageConfigName = "ai_usage_native_host.yaml"
	// aiUsageNativeHostName is the Chrome native messaging host name (manifest + registry key).
	aiUsageNativeHostName = "com.datadoghq.ai_usage_agent.native_host"
	// aiUsageScheduledTaskName is the desktop-monitor scheduled task.
	aiUsageScheduledTaskName = "Datadog AI Usage Agent"
	// aiUsageChromeRegistryKey is the machine-wide Chrome NativeMessagingHosts registration.
	aiUsageChromeRegistryKey = `HKLM:\Software\Google\Chrome\NativeMessagingHosts\` + aiUsageNativeHostName
)

type testEUDMExtensionMSI struct {
	BaseSuite
}

// TestEUDMExtensionViaMSI verifies that the eudm extension (currently the AI Usage Chrome Native
// Messaging host + desktop monitor) is installed when End User Device Monitoring is enabled at
// MSI install time (DD_INFRASTRUCTURE_MODE=end_user_device), skipped otherwise, and removed on
// uninstall. Mirrors ddot_msi_install_test.go.
func TestEUDMExtensionViaMSI(t *testing.T) {
	e2e.Run(t, &testEUDMExtensionMSI{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

// TestInstallAndUninstallEUDMExtension installs the current Agent MSI with EUDM enabled and
// asserts the eudm extension layer landed and all runtime artifacts were created, then
// uninstalls and asserts they are gone.
func (s *testEUDMExtensionMSI) TestInstallAndUninstallEUDMExtension() {
	// Act: install current Agent MSI with EUDM enabled
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-eudm-extension.log"),
		WithMSIArg("APIKEY="+installer.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
		WithMSIArg("DD_INFRASTRUCTURE_MODE=end_user_device"),
	))

	// Assert: the eudm extension layer landed under the agent package
	eudmExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", eudmExtensionName)
	s.Require().Host(s.Env().RemoteHost).DirExists(eudmExtDir, "eudm extension directory should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(eudmExtDir, aiUsageBinaryName),
		"native host binary should be present in the eudm extension layer",
	)

	// Assert: the extension hook laid down the runtime artifacts
	s.assertAIUsageInstalled()

	// Act: uninstall the Agent MSI (purges OCI packages including extensions)
	s.Require().NoError(s.Installer().Uninstall(
		WithMSILogFile("uninstall-eudm-extension.log"),
	))

	// Assert: the extension layer and its runtime artifacts are gone
	s.Require().Host(s.Env().RemoteHost).NoDirExists(eudmExtDir, "eudm extension should be removed on uninstall")
	s.Require().Host(s.Env().RemoteHost).HasNoRegistryKey(aiUsageChromeRegistryKey)
	s.assertAIUsageScheduledTask(false)
}

// TestUpgradeEnablesEUDMExtension installs a stable Agent without EUDM, then upgrades to the
// current version with EUDM enabled, and verifies the eudm extension gets installed.
func (s *testEUDMExtensionMSI) TestUpgradeEnablesEUDMExtension() {
	// Arrange: install a stable Agent without EUDM
	s.installPreviousAgentVersion()

	// Sanity: the eudm extension is not present before enabling EUDM
	eudmExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", eudmExtensionName)
	s.Require().Host(s.Env().RemoteHost).NoDirExists(eudmExtDir, "eudm extension should be absent before EUDM is enabled")

	// Act: upgrade to current version with EUDM enabled
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("upgrade-enable-eudm.log"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
		WithMSIArg("DD_INFRASTRUCTURE_MODE=end_user_device"),
	))

	// Assert: the eudm extension is installed
	s.Require().Host(s.Env().RemoteHost).DirExists(eudmExtDir, "eudm extension directory should exist after upgrade")
	s.assertAIUsageInstalled()
}

// TestInstallWithoutEUDMSkipsExtension installs the current Agent MSI without EUDM and
// verifies the eudm extension is NOT installed (the whole point of the EUDM gate).
func (s *testEUDMExtensionMSI) TestInstallWithoutEUDMSkipsExtension() {
	// Act: install current Agent MSI without EUDM
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-no-eudm.log"),
		WithMSIArg("APIKEY="+installer.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
	))

	// Assert: no eudm extension, no Chrome registration, no scheduled task
	eudmExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", eudmExtensionName)
	s.Require().Host(s.Env().RemoteHost).NoDirExists(eudmExtDir, "eudm extension should not be installed without EUDM")
	s.Require().Host(s.Env().RemoteHost).HasNoRegistryKey(aiUsageChromeRegistryKey)
	s.assertAIUsageScheduledTask(false)
}

// assertAIUsageInstalled asserts every artifact the eudm extension's AI Usage hook creates on
// install: the native host binary and Chrome host manifest in the extension layer, the generated
// config in ProgramData, the HKLM Chrome registration, and the logon-triggered scheduled task.
func (s *testEUDMExtensionMSI) assertAIUsageInstalled() {
	eudmExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", eudmExtensionName)
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(eudmExtDir, aiUsageBinaryName),
		"native host binary should be present in the extension layer",
	)
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(eudmExtDir, aiUsageNativeHostName+".json"),
		"Chrome native messaging manifest should be present in the extension layer",
	)
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(aiUsageConfigDir, aiUsageConfigName),
		"ai_usage_native_host.yaml should be generated in ProgramData",
	)
	s.Require().Host(s.Env().RemoteHost).HasRegistryKey(aiUsageChromeRegistryKey)
	s.assertAIUsageScheduledTask(true)
}

// assertAIUsageScheduledTask asserts the "Datadog AI Usage Agent" scheduled task is present
// (exists=true) or absent (exists=false).
func (s *testEUDMExtensionMSI) assertAIUsageScheduledTask(exists bool) {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`schtasks /query /TN "%s"`, aiUsageScheduledTaskName))
	if exists {
		s.Require().NoError(err, "the AI Usage desktop monitor scheduled task should exist")
	} else {
		s.Require().Error(err, "the AI Usage desktop monitor scheduled task should not exist")
	}
}

// installPreviousAgentVersion installs the stable Agent MSI as an upgrade baseline.
func (s *testEUDMExtensionMSI) installPreviousAgentVersion(opts ...MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []MsiOption{
		WithOption(WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-previous-version.log"),
		WithMSIArg("APIKEY=" + installer.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
	}
	options = append(options, opts...)
	s.Require().NoError(s.Installer().Install(options...))

	// sanity: ensure the stable version is installed
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}
