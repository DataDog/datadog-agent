// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"os"
	"testing"

	"go.yaml.in/yaml/v2"

	infraos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scenwin "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerhost "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	wincommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	wincommonagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// testInstallExeSuite is a test suite that uses the exe installer
type testInstallExeSuite struct {
	installerwindows.BaseSuite
}

// TestInstallExe tests the usage of the Datadog installer exe to install the Datadog Agent package.
//
// This test may end up being transitionary only. The exe is intended to pin/only install its own matching
// version of the Agent, but this is WIP. Once we migrate to the install script that installs the
// matching exe version this test may not be needed anymore.
func TestInstallExe(t *testing.T) {
	e2e.Run(t, &testInstallExeSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// BeforeTest sets up the test
func (s *testInstallExeSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	s.SetInstallScriptImpl(installerwindows.NewDatadogInstallExe(s.Env().RemoteHost,
		installerwindows.WithInstallScriptDevEnvOverrides("CURRENT_AGENT"),
	))
}

// TestInstallAgentPackage tests installing the Datadog Agent using the Datadog installer exe.
func (s *testInstallExeSuite) TestInstallAgentPackage() {
	// Arrange
	packageConfig, err := installerwindows.NewPackageConfig(
		installerwindows.WithPackage(s.CurrentAgentVersion().OCIPackage()),
	)
	s.Require().NoError(err)

	// Act
	output, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(map[string]string{
		"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT": packageConfig.Version,
		"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE":        packageConfig.Registry,
	}))

	// Assert
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		}).
		HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, s.CurrentAgentVersion().PackageVersion())
		}).
		WithExperimentVersionEqual("")

	wincommonagent.TestAgentHasNoWorldWritablePaths(s.T(), s.Env().RemoteHost)
}

// TestInstallAgentFails asserts various system state when the installer fails to install the Agent package (it's not available)
func (s *testInstallExeSuite) TestInstallAgentFails() {
	// Act
	_, err := s.InstallScript().Run(installerwindows.WithExtraEnvVars(map[string]string{
		"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE": "does-not-exist.internal",
	}))
	s.Require().Error(err, "expected install to fail because Agent package is not available")

	// Assert
	configDir := `C:\ProgramData\Datadog`
	s.Require().Host(s.Env().RemoteHost).
		DirExists(configDir)
	// check that config dir is protected even though MSI didn't run
	security, err := wincommon.GetSecurityInfoForPath(s.Env().RemoteHost, configDir)
	s.Require().NoError(err)
	s.Require().True(security.AreAccessRulesProtected, "config dir should be protected")
	s.Require().Equal(wincommon.GetIdentityForSID(wincommon.AdministratorsSID).GetSID(), security.Owner.GetSID(), "config dir should be owned by Administrators group")
	s.Require().Equal(wincommon.GetIdentityForSID(wincommon.AdministratorsSID).GetSID(), security.Group.GetSID(), "config dir should be grouped by Administrators group")
	// Agent is not installed so we can't grab paths from the registry keys, must provide them manually
	wincommonagent.TestHasNoWorldWritablePaths(s.T(), s.Env().RemoteHost, []string{configDir})
}

// TestConfigValuesNotOverwrittenByDefaults is a regression test for WINA-2118.
// It verifies that when an empty datadog.yaml file exists before installation,
// the installer does not overwrite it with default values from unset environment variables.
func (s *testInstallExeSuite) TestConfigValuesNotOverwrittenByDefaults() {
	// Arrange
	host := s.Env().RemoteHost
	configDir := `C:\ProgramData\Datadog`
	configPath := `C:\ProgramData\Datadog\datadog.yaml`

	// Create config directory and empty datadog.yaml
	err := host.MkdirAll(configDir)
	s.Require().NoError(err, "failed to create config directory")
	_, err = host.WriteFile(configPath, []byte{})
	s.Require().NoError(err, "failed to create empty datadog.yaml")

	// Verify the file is empty before install
	contentBefore, err := host.ReadFile(configPath)
	s.Require().NoError(err, "failed to read datadog.yaml before install")
	s.Require().Empty(contentBefore, "datadog.yaml should be empty before install")

	// Act
	// Run the installer without any config options to test that default env.Env values
	// do not overwrite config values (WINA-2118).
	output, err := s.InstallScript().Run(
		// explicitly unset some values that are always set by this Run helper method
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_API_KEY":        "",
			"DD_SITE":           "",
			"DD_REMOTE_UPDATES": "",
		}),
	)

	// Assert
	s.Require().NoErrorf(err, "failed to run installer: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))

	// Verify the datadog.yaml file has no configuration values set.
	// The installer may add comments or an empty YAML object ({}), but should not
	// write any actual configuration keys when no options are provided.
	// We use an empty config file to ensure we catch any/all options that may be written to the config
	// if they are not provided to the installer, so the test should continue working as we add new options.
	// If later we want to write defaults to the config we can update this test, but we must ensure that
	// if the option exists in datadog.yaml that it takes precedence over the default value.
	contentAfter, err := host.ReadFile(configPath)
	s.Require().NoError(err, "failed to read datadog.yaml after install")

	var configValues map[string]interface{}
	err = yaml.Unmarshal(contentAfter, &configValues)
	s.Require().NoError(err, "failed to parse datadog.yaml as YAML")
	s.Require().Empty(configValues, "datadog.yaml should have no configuration values set - default env values should not overwrite config. Got: %v", configValues)
}

// proxyEnv provisions a Windows VM (for the installer) and a Linux VM (hosting a Squid proxy)
type proxyEnv struct {
	environments.WindowsHost
	LinuxProxy *components.RemoteHost
}

// proxyEnvProvisioner provisions the two VMs
func proxyEnvProvisioner() provisioners.PulumiEnvRunFunc[proxyEnv] {
	return func(ctx *pulumi.Context, env *proxyEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// Windows host using standard WindowsHost provisioner pattern
		runParams := scenwin.GetRunParams(scenwin.WithoutAgent(), scenwin.WithoutFakeIntake())
		if err := scenwin.RunWithEnv(ctx, awsEnv, &env.WindowsHost, runParams); err != nil {
			return err
		}

		lin, err := ec2.NewVM(awsEnv, "LinuxProxyVM", ec2.WithOS(infraos.UbuntuDefault))
		if err != nil {
			return err
		}
		lin.Export(ctx, &env.LinuxProxy.HostOutput)
		return nil
	}
}

// testInstallExeProxySuite installs via the installer exe while using an HTTP(S) proxy
//
// TODO: Can't use installerwindows.BaseSuite because we have a custom env. Would need to make a lot of changes to make it work.
type testInstallExeProxySuite struct {
	e2e.BaseSuite[proxyEnv]
}

// TestInstallExeWithProxy tests installing the Datadog Agent using the Datadog installer exe with an HTTP(S) proxy.
func TestInstallExeWithProxy(t *testing.T) {
	e2e.Run(
		t,
		&testInstallExeProxySuite{},
		e2e.WithPulumiProvisioner(proxyEnvProvisioner(), nil),
	)
}

func (s *testInstallExeProxySuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	linuxHost := s.Env().LinuxProxy
	windowsHost := s.Env().RemoteHost

	// start Squid on the Linux host
	proxyHost := installerhost.New(s.T, linuxHost, infraos.UbuntuDefault, infraos.AMD64Arch)
	proxyHost.SetupProxy()
	s.T().Cleanup(func() { proxyHost.RemoveProxy() })
	proxyIP := linuxHost.HostOutput.Address
	proxyURL := fmt.Sprintf("http://%s:3128", proxyIP)

	// Configure Windows Firewall to allow outbound only to the proxy
	s.Require().NoError(wincommon.BlockAllOutboundExceptProxy(windowsHost, proxyIP, 3128))
	s.T().Cleanup(func() { wincommon.ResetOutboundPolicyAndRemoveProxyRules(windowsHost) })

	// Configure Windows system proxy
	s.Require().NoError(wincommon.SetSystemProxy(windowsHost, proxyURL))
	s.T().Cleanup(func() { wincommon.ResetSystemProxy(windowsHost) })
}

func (s *testInstallExeProxySuite) TestInstallAgentPackageWithProxy() {
	linuxHost := s.Env().LinuxProxy
	windowsHost := s.Env().RemoteHost

	// Arrange

	// Act
	// run the installer exe with proxy env vars
	proxyURL := fmt.Sprintf("http://%s:3128", linuxHost.HostOutput.Address)
	envVars := map[string]string{
		// for datadog code
		"DD_PROXY_HTTP":  proxyURL,
		"DD_PROXY_HTTPS": proxyURL,
	}
	installExe := installerwindows.NewDatadogInstallExe(windowsHost, installerwindows.WithExtraEnvVars(envVars))
	_, err := installExe.Run()
	s.Require().NoError(err)

	// Assert
	s.Require().Host(windowsHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().RuntimeConfig().
		// proxy options should be written in Agent config
		WithValueEqual("proxy.http", proxyURL).
		WithValueEqual("proxy.https", proxyURL)

	// Verify squid-proxy saw traffic to the container/installer host (configurable)
	// TODO: if we used BaseSuite we could use CurrentAgentVersion() to get the registry URL.
	//       would need to add support to install_script.go, too. it currently only supports
	//       the pipeline version.
	registryHost := os.Getenv("DD_TEST_REGISTRY_HOST")
	if registryHost == "" {
		registryHost = consts.PipelineOCIRegistry
	}
	squidLogs := linuxHost.MustExecute("sudo docker logs squid-proxy")
	s.Require().Contains(squidLogs, registryHost, "expected squid-proxy logs to include traffic to %s", registryHost)
}

func (s *testInstallExeProxySuite) Require() *suiteasserts.SuiteAssertions {
	return suiteasserts.New(s, s.BaseSuite.Require())
}
