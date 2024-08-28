// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package powershellmoduletest contains e2e tests for the Windows powershell module
package powershellmoduletest

import (
	"flag"
	"fmt"
	runneros "os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

var (
	devMode = flag.Bool("devmode", false, "enable dev mode")

	localModuleDir  string
	remoteModuleDir = "C:\\Program Files\\WindowsPowerShell\\Modules\\Datadog"
)

type vmSuite struct {
	e2e.BaseSuite[environments.Host]
}

func init() {
	// Get the absolute path to the powershell module directory
	currDir, _ := runneros.Getwd()
	localModuleDir = filepath.Join(currDir, "../../../../../powershell-module/Datadog")
}

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))))}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &vmSuite{}, suiteParams...)
}

func (v *vmSuite) TestPowershellModule() {
	v.setupTestHost()

	v.T().Run("Install module", v.installModule)
	v.T().Run("Test agent install", v.testInstallAgent751)
	v.T().Run("Test agent upgrade with dotnet tracer", v.testAgentUpgradeWithDotnetTracer)
	v.T().Run("Test failed agent downgrade", v.testFailedAgentDowngrade)
}

func (v *vmSuite) installModule(_ *testing.T) {
	vm := v.Env().RemoteHost

	err := vm.MkdirAll(remoteModuleDir)
	v.Require().NoError(err)
	vm.CopyFolder(localModuleDir, remoteModuleDir)

	out, err := vm.Execute("(Get-Command Install-DatadogAgent).Source")
	v.Require().NoError(err)
	v.Require().Equal("Datadog", strings.TrimSpace(out))
}

func (v *vmSuite) testInstallAgent751(t *testing.T) {
	vm := v.Env().RemoteHost

	params := map[string]string{
		"AgentVersion":             "7.51.0",
		"ApplicationDataDirectory": "C:\\ProgramData\\Datadog Install Test\\",
		"ProjectLocation":          "C:\\Program Files\\Datadog\\Datadog Agent Install Test\\",
		"DDAgentUsername":          "testName",
		"DDAgentPassword":          "t3stP@ssw0rd",
		"ApiKey":                   "123abc",
		"Site":                     "foo.com",
		"Hostname":                 "testVM",
		"Tags":                     "key:val",
	}

	command := "Install-DatadogAgent"
	for param, val := range params {
		command += fmt.Sprintf(" -%s '%s'", param, val)
	}

	v.T().Log(command)
	out, err := vm.Execute(command)
	v.T().Log(out)
	v.Require().NoError(err)

	// Validate that the parameters were used
	projectLocation, err := windowsAgent.GetInstallPathFromRegistry(vm)
	v.Require().NoError(err)
	v.Assert().Equal(params["ProjectLocation"], projectLocation)

	applicationDataDirectory, err := windowsAgent.GetConfigRootFromRegistry(vm)
	v.Require().NoError(err)
	v.Assert().Equal(params["ApplicationDataDirectory"], applicationDataDirectory)

	configKeysByParameterName := map[string]string{
		"ApiKey":   "api_key",
		"Site":     "site",
		"Hostname": "hostname",
		"Tags":     "tags",
	}
	for paramName, configKey := range configKeysByParameterName {
		configVal, err := v.getConfiguredValue(applicationDataDirectory, configKey)
		v.Require().NoError(err)
		configVal = strings.TrimSpace(configVal)

		v.Assert().Equal(params[paramName], configVal)
	}

	// Validate that the correct agent version is running
	testClient := common.NewWindowsTestClient(v, vm)
	installedVersion, err := testClient.GetAgentVersion()
	v.Assert().NoError(err)
	windowsAgent.TestAgentVersion(t, params["AgentVersion"], installedVersion)
}

func (v *vmSuite) testAgentUpgradeWithDotnetTracer(t *testing.T) {
	vm := v.Env().RemoteHost

	agentVersion := "7.52.0"

	// In order to test the 'AgentInstallerPath' parameter, we'll download the installer directly to the test VM and pass its location to the script
	v.T().Logf("Downloading agent %s", agentVersion)
	url := fmt.Sprintf("https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-%s.msi", agentVersion)
	remoteInstallerPath := "C:\\Users\\Administrator\\ddagentLatest.msi"
	vm.MustExecute(fmt.Sprintf("(New-Object System.Net.WebClient).DownloadFile(\"%s\", \"%s\")", url, remoteInstallerPath))

	newAPIKey := "newApiKey"
	command := fmt.Sprintf("Install-DatadogAgent -AgentInstallerPath '%s' -ApiKey '%s' -ApmInstrumentationEnabled", remoteInstallerPath, newAPIKey)

	v.T().Log(command)
	out, err := vm.Execute(command)
	v.T().Log(out)
	v.Require().NoError(err)
	v.Assert().Contains(out, "WARNING: A datadog.yaml file already exists")

	// Validate that the API Key parameter was ignored
	applicationDataDirectory, err := windowsAgent.GetConfigRootFromRegistry(vm)
	v.Require().NoError(err)

	configuredAPIKey, err := v.getConfiguredValue(applicationDataDirectory, "api_key")
	v.Require().NoError(err)
	v.Assert().NotEqual(newAPIKey, configuredAPIKey)

	// Validate that the correct agent version is running
	testClient := common.NewWindowsTestClient(v, vm)
	installedVersion, err := testClient.GetAgentVersion()
	v.Assert().NoError(err)
	windowsAgent.TestAgentVersion(t, agentVersion, installedVersion)

	// Validate the .NET Tracer install succeeded
	installPath, err := windowsCommon.GetRegistryValue(vm, "HKLM:\\SOFTWARE\\Datadog\\Datadog .NET Tracer 64-bit", "InstallPath")
	v.Require().NoError(err)
	v.Require().Equal(installPath, "C:\\Program Files\\Datadog\\.NET Tracer\\")
}

func (v *vmSuite) testFailedAgentDowngrade(_ *testing.T) {
	vm := v.Env().RemoteHost

	downgradeURL := "https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-7.50.0.msi"
	installLog := "C:\\Users\\Administrator\\ddagentInstall.log"
	command := fmt.Sprintf("Install-DatadogAgent -AgentInstallerURL %s -AgentInstallLogPath %s", downgradeURL, installLog)

	v.T().Log(command)
	_, err := vm.Execute(command)
	v.T().Log(err)

	// Validate that there was an error, and that the error message contains the location of the install log
	v.Require().ErrorContains(err, installLog)
}

func (v *vmSuite) setupTestHost() {
	vm := v.Env().RemoteHost

	// Install powershell-yaml
	vm.MustExecute("Install-PackageProvider NuGet -Force")
	vm.MustExecute("Set-PSRepository PSGallery -InstallationPolicy Trusted")
	vm.MustExecute("Install-Module powershell-yaml -Repository PSGallery")
}

func (v *vmSuite) getConfiguredValue(applicationDataDirectory, keyName string) (string, error) {
	vm := v.Env().RemoteHost
	return vm.Execute(fmt.Sprintf("$(Get-Content -path '%s\\datadog.yaml' | ConvertFrom-Yaml).%s", applicationDataDirectory, keyName))
}
