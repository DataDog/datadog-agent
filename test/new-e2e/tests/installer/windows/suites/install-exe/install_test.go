// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerhost "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	wincommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	infraos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
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
		params := winawshost.GetProvisionerParams(winawshost.WithoutAgent(), winawshost.WithoutFakeIntake())
		if err := winawshost.Run(ctx, &env.WindowsHost, awsEnv, params); err != nil {
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
