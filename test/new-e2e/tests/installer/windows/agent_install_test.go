// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"
)

type testAgentInstallSuite struct {
	BaseSuite
}

// TestAgentInstalls tests the usage of the Datadog installer to install the Datadog Agent package.
func TestAgentInstalls(t *testing.T) {
	e2e.Run(t, &testAgentInstallSuite{},
		e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()))
}

// TestInstallAgentPackage covers Fleet removal retaining disabled services, then CLI
// purge deleting those registrations after the MSI product has been unregistered.
func (s *testAgentInstallSuite) TestInstallAgentPackage() {
	host := s.Env().RemoteHost
	var tempFile, binaryPath string
	copiedCLI := false
	defer func() {
		if copiedCLI {
			output, err := s.Installer().Purge()
			s.Assert().NoErrorf(err, "cleanup purge: %s", output)
		} else if _, err := common.GetProductCodeByName(host, "Datadog Agent"); err == nil {
			// Setup or copying the CLI may fail after registering the MSI.
			s.Assert().NoError(s.Installer().Uninstall(WithMSILogFile("cleanup-msi.log")))
		}
		if binaryPath != "" {
			s.Assert().NoError(host.RemoveAll(binaryPath))
		}
		if tempFile != "" {
			s.Assert().NoError(host.Remove(tempFile))
		}
	}()

	// The provisioner starts with no Agent or CLI. Bootstrap the current artifacts.
	output, err := s.InstallScript().Run(WithInstallScriptDevEnvOverrides("CURRENT_AGENT"))
	s.Require().NoErrorf(err, "failed to install Agent via setup: %s", output)
	s.Require().Host(host).HasARunningDatadogAgentService()
	productCode, err := common.GetProductCodeByName(host, "Datadog Agent")
	s.Require().NoError(err)

	// Save a working CLI before Fleet removal deletes the installed binaries.
	tempFile, err = common.GetTemporaryFile(host)
	s.Require().NoError(err)
	binaryPath = tempFile + ".exe"
	_, err = host.Execute(fmt.Sprintf(`Copy-Item -Force -Path "%s" -Destination "%s"`, consts.BinaryPath, binaryPath))
	s.Require().NoError(err)
	s.Installer().SetBinaryPath(binaryPath)
	copiedCLI = true

	s.removeAgentPackage()
	exists, err := common.RegistryKeyExists(host, `HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\`+productCode)
	s.Require().NoError(err)
	s.Require().False(exists, "Fleet removal must unregister the MSI before purge")

	services := []string{
		"datadogagent", "datadog-trace-agent", "datadog-process-agent",
		"datadog-security-agent", "datadog-system-probe", "datadog-agent-action",
		"dd-procmgr-service", "Datadog Installer",
	}
	for _, name := range services {
		config, err := common.GetServiceConfig(host, name)
		s.Require().NoError(err)
		s.Require().Equal(common.SERVICE_DISABLED, config.StartType, "%s must be retained disabled", name)
		status, err := common.GetServiceStatus(host, name)
		s.Require().NoError(err)
		s.Require().Equal("Stopped", status, name)
	}

	for attempt := 0; attempt < 2; attempt++ {
		output, err := s.Installer().Purge()
		s.Require().NoErrorf(err, "purge attempt %d: %s", attempt+1, output)
		s.EventuallyWithT(func(c *assert.CollectT) {
			for _, name := range services {
				exists, err := common.RegistryKeyExists(host, `HKLM:\SYSTEM\CurrentControlSet\Services\`+name)
				require.NoError(c, err)
				assert.False(c, exists, "%s must be deleted after purge", name)
			}
		}, time.Minute, time.Second)
	}
}

// TestSetupScriptInstallInfo tests that the Fleet Automation setup script correctly writes install_info with the setup script tool.
func (s *testAgentInstallSuite) TestSetupScriptInstallInfo() {
	s.Run("Install via setup script", func() {
		s.installAgentViaSetupScript()
		s.Run("Verify install_info", s.verifySetupScriptInstallInfo)
		s.Run("Uninstall", s.uninstallAgentWithMSI)
	})
}

func (s *testAgentInstallSuite) installAgent() {
	// Arrange

	// Act
	output, err := s.Installer().InstallPackage(consts.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
}

func (s *testAgentInstallSuite) installAgentViaSetupScript() {
	installExe := NewDatadogInstallExe(s.Env().RemoteHost)

	// Act - This calls the same path as Fleet Automation setup script: installer.exe setup --flavor default
	output, err := installExe.Run()

	// Assert
	s.Require().NoErrorf(err, "failed to install via setup script: %s", output)
	s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
}

func (s *testAgentInstallSuite) verifySetupScriptInstallInfo() {
	// Arrange
	installInfoPath := "C:\\ProgramData\\Datadog\\install_info"

	// Act
	installInfoContent, err := s.Env().RemoteHost.ReadFile(installInfoPath)

	// Assert
	s.Require().NoError(err, "should be able to read install_info file")
	s.Require().Contains(string(installInfoContent), "tool: installer",
		"install_info should contain the setup script installation method")
}

func (s *testAgentInstallSuite) removeAgentPackage() {
	// Arrange

	// Act
	output, err := s.Installer().RemovePackage(consts.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to remove the Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(consts.GetStableDirFor(consts.AgentPackage),
			"the package directory should be removed")
}

func (s *testAgentInstallSuite) TestRemoveAgentAfterMSIUninstall() {
	// Arrange
	s.installAgent()
	s.uninstallAgentWithMSI()

	// Act

	// Assert
	s.removeAgentPackage()
	s.Require().Host(s.Env().RemoteHost).HasNoDatadogAgentService()
}

func (s *testAgentInstallSuite) uninstallAgentWithMSI() {
	// Arrange

	// Act
	err := windowsAgent.UninstallAgent(s.Env().RemoteHost,
		filepath.Join(s.SessionOutputDir(), "uninstall.log"),
	)

	// Assert
	s.Require().NoErrorf(err, "failed to uninstall the Datadog Agent package")
	// we default to purge everything, so we don't need to check for the package directory
}
