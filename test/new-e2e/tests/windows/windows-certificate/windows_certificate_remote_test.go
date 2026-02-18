// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windowscertificate

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/certificatehost"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/stretchr/testify/assert"
)

const (
	TestUser           = "TestUser"
	TestPassword       = "Test1234#"
	agentBinaryPath    = `C:\Program Files\Datadog\Datadog Agent\bin\agent.exe`
	checkConfigPath    = `C:\ProgramData\Datadog\conf.d\windows_certificate.d\conf.yaml`
	checkName          = "windows_certificate"
	agentStatusTimeout = 5 * time.Minute
	checkRunTimeout    = 3 * time.Minute
	retryInterval      = 10 * time.Second
)

type multiVMEnv struct {
	AgentHost       *components.RemoteHost
	CertificateHost *components.RemoteHost
}

func multiVMEnvProvisioner() provisioners.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		agentHost, err := ec2.NewVM(awsEnv, "agenthost", ec2.WithOS(os.WindowsServerDefault))
		if err != nil {
			return err
		}
		agentHost.Export(ctx, &env.AgentHost.HostOutput)

		certificateHost, err := ec2.NewVM(awsEnv, "certificatehost", ec2.WithOS(os.WindowsServerDefault))
		if err != nil {
			return err
		}
		certificateHost.Export(ctx, &env.CertificateHost.HostOutput)

		// Setup Windows Defender to be disabled
		defenderManager, err := defender.NewDefender(awsEnv.CommonEnvironment, certificateHost,
			defender.WithDefenderDisabled())
		if err != nil {
			return err
		}

		// Setup the certificate host with required configuration
		// This depends on Defender being configured first
		_, err = certificatehost.NewCertificateHost(awsEnv.CommonEnvironment, certificateHost,
			certificatehost.WithUser(TestUser, TestPassword),
			certificatehost.WithSelfSignedCert("CN=test_cert"),
			certificatehost.WithPulumiResourceOptions(pulumi.DependsOn(defenderManager.Resources)))
		if err != nil {
			return err
		}

		return nil
	}
}

type multiVMSuite struct {
	e2e.BaseSuite[multiVMEnv]
}

func TestRemoteCertificates(t *testing.T) {
	flake.Mark(t)
	t.Parallel()
	e2e.Run(t, &multiVMSuite{}, e2e.WithPulumiProvisioner(multiVMEnvProvisioner(), nil))
}

// waitForServiceRunning waits for a Windows service to be in Running state
func (v *multiVMSuite) waitForServiceRunning(host *components.RemoteHost, serviceName string, timeout time.Duration) {
	v.EventuallyWithT(func(c *assert.CollectT) {
		output, err := windowsCommon.GetServiceStatus(host, serviceName)
		if err != nil {
			v.T().Logf("Getting %s status failed: %v", serviceName, err)
		}
		assert.NoError(c, err)
		assert.Contains(c, output, "Running")
		v.T().Logf("%s status: %s", serviceName, output)
	}, timeout, retryInterval)
}

// writeCheckConfigAndRestart writes the check configuration and restarts the agent
func (v *multiVMSuite) writeCheckConfigAndRestart(host *components.RemoteHost, config string) {
	v.T().Logf("Check config: %s", config)
	_, err := host.WriteFile(checkConfigPath, []byte(config))
	v.Require().NoError(err)

	err = restartAgent(v, host)
	v.Require().NoError(err)
}

// waitForAgentCheckRunning waits for the agent to be running and the specified check to be loaded
func (v *multiVMSuite) waitForAgentCheckRunning(host *components.RemoteHost) {
	cmd := fmt.Sprintf(`&'%s' status`, agentBinaryPath)
	v.EventuallyWithT(func(c *assert.CollectT) {
		output, err := host.Execute(cmd)
		assert.NoError(c, err)
		assert.Contains(c, output, checkName)
		v.T().Logf("Agent status: %s", output)
	}, agentStatusTimeout, retryInterval)
}

// runCheckAndWaitForMetric runs the certificate check and waits for the expected metric to appear
func (v *multiVMSuite) runCheckAndWaitForMetric(host *components.RemoteHost, expectedMetric string) string {
	cmdCheck := fmt.Sprintf(`&'%s' check %s`, agentBinaryPath, checkName)
	var output string
	v.EventuallyWithT(func(c *assert.CollectT) {
		var err error
		output, err = host.Execute(cmdCheck)
		assert.NoError(c, err)
		assert.Contains(c, output, expectedMetric)
		assert.NotContains(c, output, "Access is denied")
		v.T().Logf("Check output: %s", output)
	}, checkRunTimeout, retryInterval)
	return output
}

func (v *multiVMSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()

	agentHost := v.Env().AgentHost
	certificateHost := v.Env().CertificateHost

	// Wait for LanmanServer to be running
	v.waitForServiceRunning(certificateHost, "LanmanServer", 10*time.Minute)

	// Wait for RemoteRegistry service to be running on certificate host
	v.waitForServiceRunning(certificateHost, "RemoteRegistry", 5*time.Minute)

	// Verify firewall rules for Remote Registry are enabled
	v.T().Logf("Verifying firewall rules on certificate host")
	firewallCheck := `Get-NetFirewallRule -DisplayGroup "Remote Service Management" -Enabled True -ErrorAction SilentlyContinue | Select-Object -First 5 -ExpandProperty DisplayName`
	output, err := certificateHost.Execute(firewallCheck)
	if err != nil {
		v.T().Logf("Warning: Could not verify firewall rules: %v", err)
	} else {
		v.T().Logf("Enabled firewall rules: %s", output)
	}

	// Verify registry permissions are set on the certificate host
	v.T().Logf("Verifying registry permissions for remote access")
	aclVerifyCmd := fmt.Sprintf(`
		$computerName = $env:COMPUTERNAME
		$identity = "$computerName\%s"
		$winregAcl = Get-Acl "HKLM:\SYSTEM\CurrentControlSet\Control\SecurePipeServers\winreg"
		$certAcl = Get-Acl "HKLM:\SOFTWARE\Microsoft\SystemCertificates"
		$winregRule = $winregAcl.Access | Where-Object { $_.IdentityReference -eq $identity }
		$certRule = $certAcl.Access | Where-Object { $_.IdentityReference -eq $identity }
		if ($winregRule -and $certRule) {
			Write-Output "SUCCESS: Registry permissions configured"
		} else {
			Write-Output "FAILED: Missing permissions"
		}
	`, TestUser)
	aclOutput, err := certificateHost.Execute(aclVerifyCmd)
	v.Require().NoError(err)
	v.T().Logf("Permission check: %s", aclOutput)
	v.Require().Contains(aclOutput, "SUCCESS", "Registry permissions must be configured")

	// Install the agent
	agentPackage, err := windowsAgent.GetPackageFromEnv()
	v.Require().NoError(err)
	v.T().Logf("Using Agent: %#v", agentPackage)
	_, err = windowsAgent.InstallAgent(agentHost,
		windowsAgent.WithPackage(agentPackage))
	v.Require().NoError(err)
}

func (v *multiVMSuite) TestGetRemoteCertificate() {
	agentHost := v.Env().AgentHost
	certificateHost := v.Env().CertificateHost

	// Configure the check for MY certificate store
	checkConfig := fmt.Sprintf(`
init_config:
instances:
  - certificate_store: MY
    server: %s
    username: %s
    password: %s`, certificateHost.HostOutput.Address, TestUser, TestPassword)

	v.writeCheckConfigAndRestart(agentHost, checkConfig)
	v.waitForAgentCheckRunning(agentHost)

	// Run the check and wait for metrics
	output := v.runCheckAndWaitForMetric(agentHost, "windows_certificate.days_remaining")

	// Verify all expected metrics and tags
	v.Require().Contains(output, "windows_certificate.days_remaining")
	v.Require().Contains(output, "windows_certificate.cert_expiration")
	v.Require().Contains(output, `"certificate_store:MY"`)
	v.Require().Contains(output, fmt.Sprintf(`"server:%s"`, certificateHost.HostOutput.Address))
	v.Require().Contains(output, `"subject_CN:test_cert"`)
}

func (v *multiVMSuite) TestGetRemoteCertificateWithCrl() {
	agentHost := v.Env().AgentHost
	certificateHost := v.Env().CertificateHost

	// Configure the check for CA certificate store with CRL monitoring
	checkConfig := fmt.Sprintf(`
init_config:
instances:
  - certificate_store: CA
    server: %s
    username: %s
    password: %s
    enable_crl_monitoring: true`, certificateHost.HostOutput.Address, TestUser, TestPassword)

	v.writeCheckConfigAndRestart(agentHost, checkConfig)
	v.waitForAgentCheckRunning(agentHost)

	// Run the check and wait for CRL metrics
	output := v.runCheckAndWaitForMetric(agentHost, "windows_certificate.crl_days_remaining")

	// Verify all expected CRL metrics and tags
	v.Require().Contains(output, "windows_certificate.crl_days_remaining")
	v.Require().Contains(output, "windows_certificate.crl_expiration")
	v.Require().Contains(output, `"certificate_store:CA"`)
	v.Require().Contains(output, fmt.Sprintf(`"server:%s"`, certificateHost.HostOutput.Address))
	v.Require().Contains(output, `"crl_issuer_OU:VeriSign Commercial Software Publishers CA"`)
}

func restartAgent(v *multiVMSuite, host *components.RemoteHost) error {
	// Ensure the agent is running before restarting it
	v.waitForServiceRunning(host, "datadogagent", 10*time.Minute)

	// Restart the service
	return windowsCommon.RestartService(host, "datadogagent")
}
