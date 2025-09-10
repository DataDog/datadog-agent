// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	installerhost "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
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
	s.SetInstallScriptImpl(installerwindows.NewDatadogInstallExe(s.Env(),
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
	WindowsVM  *components.RemoteHost
	LinuxProxy *components.RemoteHost
}

// proxyEnvProvisioner provisions the two VMs
func proxyEnvProvisioner() provisioners.PulumiEnvRunFunc[proxyEnv] {
	return func(ctx *pulumi.Context, env *proxyEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		win, err := ec2.NewVM(awsEnv, "WindowsVM", ec2.WithOS(infraos.WindowsDefault))
		if err != nil {
			return err
		}
		win.Export(ctx, &env.WindowsVM.HostOutput)

		lin, err := ec2.NewVM(awsEnv, "LinuxProxyVM", ec2.WithOS(infraos.UbuntuDefault))
		if err != nil {
			return err
		}
		lin.Export(ctx, &env.LinuxProxy.HostOutput)
		return nil
	}
}

// testInstallExeProxySuite installs via the installer exe while using an HTTP(S) proxy
type testInstallExeProxySuite struct {
	e2e.BaseSuite[proxyEnv]
}

// TestInstallExeWithProxy provisions a Windows host and a Linux proxy host, configures the proxy, then installs via the exe using that proxy
func TestInstallExeWithProxy(t *testing.T) {
	e2e.Run(
		t,
		&testInstallExeProxySuite{},
		e2e.WithPulumiProvisioner(proxyEnvProvisioner(), nil),
	)
}

func (s *testInstallExeProxySuite) TestInstallAgentPackageWithProxy() {
	// Arrange: start Squid on the Linux host
	linuxHost := installerhost.New(s.T, s.Env().LinuxProxy, infraos.UbuntuDefault, infraos.AMD64Arch)
	linuxHost.SetupProxy()
	// s.T().Cleanup(func() { linuxHost.RemoveProxy() })

	// Build proxy env vars for Windows installer
	proxyURL := fmt.Sprintf("http://%s:3128", s.Env().LinuxProxy.HostOutput.Address)
	proxyIP := s.Env().LinuxProxy.HostOutput.Address
	envVars := map[string]string{
		"DD_PROXY_HTTP":     proxyURL,
		"DD_PROXY_HTTPS":    proxyURL,
		"DD_API_KEY":        "deadbeefdeadbeefdeadbeefdeadbeef",
		"DD_SITE":           "datadoghq.com",
		"DD_REMOTE_UPDATES": "true",
	}

	// Configure Windows Firewall to allow outbound only to the proxy (80/443), then install via proxy
	fw := fmt.Sprintf(`
		$proxyIp = '%s'
		# Block all outbound by default, then allow only the proxy for 80/443
		Set-NetFirewallProfile -Profile Domain,Public,Private -DefaultOutboundAction Block
		New-NetFirewallRule -DisplayName 'AllowProxy80' -Direction Outbound -Action Allow -RemoteAddress $proxyIp -Protocol TCP -RemotePort 80
		New-NetFirewallRule -DisplayName 'AllowProxy443' -Direction Outbound -Action Allow -RemoteAddress $proxyIp -Protocol TCP -RemotePort 443
	`, proxyIP)
	_, err := s.Env().WindowsVM.Execute(fw)
	s.Require().NoError(err)

	// Ensure firewall rules are cleaned up even if the test fails
	s.T().Cleanup(func() {
		_, _ = s.Env().WindowsVM.Execute(`Remove-NetFirewallRule -DisplayName 'AllowProxy80' -ErrorAction SilentlyContinue; Remove-NetFirewallRule -DisplayName 'AllowProxy443' -ErrorAction SilentlyContinue; Set-NetFirewallProfile -Profile Domain,Public,Private -DefaultOutboundAction Allow`)
	})

	ps := `
		[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
		$temp = [System.IO.Path]::GetTempFileName() + '.exe';
		$uri  = 'https://install.datadoghq.com/datadog-installer-x86_64.exe';
		(New-Object System.Net.WebClient).DownloadFile($uri, $temp);
		& $temp setup --flavor default
	`
	output, err := s.Env().WindowsVM.Execute(ps, client.WithEnvVariables(envVars))
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}

	// Assert: installer and agent services are running
	_, err = s.Env().WindowsVM.Execute(`if ((Get-Service -Name 'Datadog Installer').Status -ne 'Running') { throw 'Installer not running' }`)
	s.Require().NoError(err)
	_, err = s.Env().WindowsVM.Execute(`if ((Get-Service -Name 'datadogagent').Status -ne 'Running') { throw 'Agent not running' }`)
	s.Require().NoError(err)

	// Verify squid-proxy saw traffic to the container/installer host (configurable)
	registryHost := os.Getenv("DD_TEST_REGISTRY_HOST")
	if registryHost == "" {
		registryHost = "install.datadoghq.com"
	}
	var logs string
	found := false
	for i := 0; i < 30; i++ {
		l, _ := s.Env().LinuxProxy.Execute("sudo docker logs --since 10m squid-proxy | cat")
		logs = l
		if strings.Contains(logs, registryHost) {
			found = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	s.Require().True(found, "expected squid-proxy logs to include traffic to %s; logs:\n%s", registryHost, logs)
}
