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
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	// aiUsageExtensionName is the fleet installer extension name.
	aiUsageExtensionName = "ai-usage"
	// aiUsageBinaryName is the native host binary shipped in the extension layer.
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

type testAIUsageExtensionMSI struct {
	BaseSuite
}

// TestAIUsageExtensionViaMSI verifies that the ai-usage extension (AI Usage Chrome Native
// Messaging host + desktop monitor) is installed when End User Device Monitoring is enabled at
// MSI install time (DD_INFRASTRUCTURE_MODE=end_user_device), skipped otherwise, and removed on
// uninstall. Mirrors ddot_msi_install_test.go.
func TestAIUsageExtensionViaMSI(t *testing.T) {
	e2e.Run(t, &testAIUsageExtensionMSI{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

// TestInstallAndUninstallAIUsageExtension installs the current Agent MSI with EUDM enabled and
// asserts the ai-usage extension layer landed and all runtime artifacts were created, then
// uninstalls and asserts they are gone.
func (s *testAIUsageExtensionMSI) TestInstallAndUninstallAIUsageExtension() {
	// Act: install current Agent MSI with EUDM enabled
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-ai-usage-extension.log"),
		WithMSIArg("APIKEY="+installer.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
		WithMSIArg("DD_INFRASTRUCTURE_MODE=end_user_device"),
	))

	// Assert: the ai-usage extension layer landed under the agent package
	aiUsageExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", aiUsageExtensionName)
	s.Require().Host(s.Env().RemoteHost).DirExists(aiUsageExtDir, "ai-usage extension directory should exist")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(aiUsageExtDir, aiUsageBinaryName),
		"native host binary should be present in the ai-usage extension layer",
	)

	// Assert: the extension hook laid down the runtime artifacts
	s.assertAIUsageInstalled()

	// Act: uninstall the Agent MSI (purges OCI packages including extensions)
	s.Require().NoError(s.Installer().Uninstall(
		WithMSILogFile("uninstall-ai-usage-extension.log"),
	))

	// Assert: the extension layer and its runtime artifacts are gone
	s.Require().Host(s.Env().RemoteHost).NoDirExists(aiUsageExtDir, "ai-usage extension should be removed on uninstall")
	s.Require().Host(s.Env().RemoteHost).HasNoRegistryKey(aiUsageChromeRegistryKey)
	s.assertAIUsageScheduledTask(false)
}

// TestUpgradeEnablesAIUsageExtension installs a stable Agent without EUDM, then upgrades to the
// current version with EUDM enabled, and verifies the ai-usage extension gets installed.
func (s *testAIUsageExtensionMSI) TestUpgradeEnablesAIUsageExtension() {
	// Arrange: install a stable Agent without EUDM
	s.installPreviousAgentVersion()

	// Sanity: the ai-usage extension is not present before enabling EUDM
	aiUsageExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", aiUsageExtensionName)
	s.Require().Host(s.Env().RemoteHost).NoDirExists(aiUsageExtDir, "ai-usage extension should be absent before EUDM is enabled")

	// Act: upgrade to current version with EUDM enabled
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("upgrade-enable-ai-usage.log"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
		WithMSIArg("DD_INFRASTRUCTURE_MODE=end_user_device"),
	))

	// Assert: the ai-usage extension is installed
	s.Require().Host(s.Env().RemoteHost).DirExists(aiUsageExtDir, "ai-usage extension directory should exist after upgrade")
	s.assertAIUsageInstalled()
}

// TestInstallWithoutEUDMSkipsAIUsageExtension installs the current Agent MSI without EUDM and
// verifies the ai-usage extension is NOT installed (the whole point of the EUDM gate).
func (s *testAIUsageExtensionMSI) TestInstallWithoutEUDMSkipsAIUsageExtension() {
	// Act: install current Agent MSI without EUDM
	s.Require().NoError(s.Installer().Install(
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-no-eudm.log"),
		WithMSIArg("APIKEY="+installer.GetAPIKey()),
		WithMSIArg("SITE=datadoghq.com"),
		WithMSIArg("DD_INSTALLER_REGISTRY_URL="+consts.PipelineOCIRegistry),
	))

	// Assert: no ai-usage extension, no Chrome registration, no scheduled task
	aiUsageExtDir := filepath.Join(consts.GetStableDirFor(consts.AgentPackage), "ext", aiUsageExtensionName)
	s.Require().Host(s.Env().RemoteHost).NoDirExists(aiUsageExtDir, "ai-usage extension should not be installed without EUDM")
	s.Require().Host(s.Env().RemoteHost).HasNoRegistryKey(aiUsageChromeRegistryKey)
	s.assertAIUsageScheduledTask(false)
}

// assertAIUsageInstalled asserts every artifact the ai-usage extension hook creates on install:
// the native host binary copied into the Agent bin dir, the generated config, the Chrome host
// manifest, the HKLM Chrome registration, and the logon-triggered scheduled task.
func (s *testAIUsageExtensionMSI) assertAIUsageInstalled() {
	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err)

	agentBinDir := filepath.Join(installPath, "bin", "agent")
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(agentBinDir, aiUsageBinaryName),
		"native host binary should be copied into the Agent bin directory",
	)
	s.Require().Host(s.Env().RemoteHost).FileExists(
		filepath.Join(agentBinDir, "dist", aiUsageNativeHostName+".json"),
		"Chrome native messaging manifest should be present",
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
func (s *testAIUsageExtensionMSI) assertAIUsageScheduledTask(exists bool) {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`schtasks /query /TN "%s"`, aiUsageScheduledTaskName))
	if exists {
		s.Require().NoError(err, "the AI Usage desktop monitor scheduled task should exist")
	} else {
		s.Require().Error(err, "the AI Usage desktop monitor scheduled task should not exist")
	}
}

// installPreviousAgentVersion installs the stable Agent MSI as an upgrade baseline.
func (s *testAIUsageExtensionMSI) installPreviousAgentVersion(opts ...MsiOption) {
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
