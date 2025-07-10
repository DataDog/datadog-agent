// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windowscertificate

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/stretchr/testify/assert"
)

const (
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
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

		agentHost, err := ec2.NewVM(awsEnv, "agenthost", ec2.WithOS(os.WindowsDefault))
		if err != nil {
			return err
		}
		agentHost.Export(ctx, &env.AgentHost.HostOutput)

		certificateHost, err := ec2.NewVM(awsEnv, "certificatehost", ec2.WithOS(os.WindowsDefault))
		if err != nil {
			return err
		}
		certificateHost.Export(ctx, &env.CertificateHost.HostOutput)

		return nil
	}
}

type multiVMSuite struct {
	e2e.BaseSuite[multiVMEnv]
}

func TestRemoteCertificates(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &multiVMSuite{}, e2e.WithPulumiProvisioner(multiVMEnvProvisioner(), nil))
}

func (v *multiVMSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()

	agentHost := v.Env().AgentHost
	certificateHost := v.Env().CertificateHost

	createTestUser := fmt.Sprintf("net user %s %s /add", TestUser, TestPassword)
	addAdmingroup := fmt.Sprintf("net localgroup administrators %s /add", TestUser)
	selfSignedCert := `New-SelfSignedCertificate -Subject "CN=test_cert" ` +
		`-CertStoreLocation "Cert:\\LocalMachine\\My" ` +
		`-KeyExportPolicy Exportable -KeySpec Signature ` +
		`-KeyLength 2048 -KeyAlgorithm RSA -HashAlgorithm SHA256`

	createTestUserOut, err := certificateHost.Execute(createTestUser)
	v.Require().NoError(err)
	v.Require().NotEmpty(createTestUserOut)

	addAdmingroupOut, err := certificateHost.Execute(addAdmingroup)
	v.Require().NoError(err)
	v.Require().NotEmpty(addAdmingroupOut)

	err = windowsCommon.DisableDefender(certificateHost)
	v.Require().NoError(err)

	// This will require a reboot to apply the policy
	// Reboot will be done once everything else is configured on certificateHost
	err = windowsCommon.SetNewItemDWORDProperty(certificateHost, `HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`,
		"LocalAccountTokenFilterPolicy", 1)
	v.Require().NoError(err)

	_, err = certificateHost.Execute(`New-NetFirewallRule -DisplayName "Allow SMB TCP 445" -Direction Inbound -Protocol TCP -LocalPort 445 -Action Allow`)
	v.Require().NoError(err)

	err = windowsCommon.StartService(certificateHost, "RemoteRegistry")
	v.Require().NoError(err)

	selfSignedCertOut, err := certificateHost.Execute(selfSignedCert)
	v.Require().NoError(err)
	v.Require().NotEmpty(selfSignedCertOut)

	// When setting, LocalAccountTokenFilterPolicy, we need to reboot the host to apply the policy
	err = RebootHost(certificateHost)
	v.Require().NoError(err)

	// Reconnect to the host
	err = certificateHost.Reconnect()
	v.Require().NoError(err)

	// Start the LanmanServer to ensure that the Agent can connect to the IPC$ share
	err = windowsCommon.StartService(certificateHost, "LanmanServer")
	v.Require().NoError(err)

	// Wait for the LanmanServer service to be running
	v.EventuallyWithT(func(c *assert.CollectT) {
		output, err := windowsCommon.GetServiceStatus(certificateHost, "LanmanServer")
		if err != nil {
			v.T().Logf("Getting LanmanServer status failed %v", err)
		}
		assert.NoError(c, err)
		assert.Contains(c, output, "Running")
		v.T().Logf("LanmanServer status: %s", output)
	}, 10*time.Minute, 10*time.Second)

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

	checkConfig := fmt.Sprintf(`
init_config:
instances:
  - certificate_store: MY
    server: %s
    username: %s
    password: %s`, certificateHost.HostOutput.Address, TestUser, TestPassword)
	v.T().Logf("Check config: %s", checkConfig)

	_, err := agentHost.WriteFile("C:\\ProgramData\\Datadog\\conf.d\\windows_certificate.d\\conf.yaml", []byte(checkConfig))
	v.Require().NoError(err)

	err = windowsCommon.RestartService(agentHost, "datadogagent")
	v.Require().NoError(err)

	agent := `C:\Program Files\Datadog\Datadog Agent\bin\agent.exe`
	cmd := fmt.Sprintf(`&'%s' status`, agent)

	// Wait for the agent to be running and check that the windows_certificate check is running
	v.EventuallyWithT(func(c *assert.CollectT) {
		output, err := agentHost.Execute(cmd)
		assert.NoError(c, err)
		assert.Contains(c, output, "windows_certificate")
		v.T().Logf("Agent status: %s", output)
	}, 5*time.Minute, 10*time.Second)

	cmdCheck := fmt.Sprintf(`&'%s' check windows_certificate`, agent)
	certStoreTag := `"certificate_store:MY"`
	serverTag := fmt.Sprintf(`"server:%s"`, certificateHost.HostOutput.Address)
	subjectCNTag := `"subject_CN:test_cert"`
	output, err := agentHost.Execute(cmdCheck)
	v.Require().NoError(err)
	v.T().Logf("Check output: %s", output)

	// Assert that the check output returns the metric and service check
	v.Require().Contains(output, "windows_certificate.days_remaining")
	v.Require().Contains(output, "windows_certificate.cert_expiration")

	// Assert that the check output returns the correct metric tags
	v.Require().Contains(output, certStoreTag)
	v.Require().Contains(output, serverTag)
	v.Require().Contains(output, subjectCNTag)

}

func (v *multiVMSuite) TestGetRemoteCertificateWithCrl() {
	agentHost := v.Env().AgentHost
	certificateHost := v.Env().CertificateHost
	checkConfig := fmt.Sprintf(`
init_config:
instances:
  - certificate_store: CA
    server: %s
    username: %s
    password: %s
    enable_crl_monitoring: true`, certificateHost.HostOutput.Address, TestUser, TestPassword)
	v.T().Logf("Check config: %s", checkConfig)

	_, err := agentHost.WriteFile("C:\\ProgramData\\Datadog\\conf.d\\windows_certificate.d\\conf.yaml", []byte(checkConfig))
	v.Require().NoError(err)

	err = windowsCommon.RestartService(agentHost, "datadogagent")
	v.Require().NoError(err)

	agent := `C:\Program Files\Datadog\Datadog Agent\bin\agent.exe`
	cmd := fmt.Sprintf(`&'%s' status`, agent)

	// Wait for the agent to be running and check that the windows_certificate check is running
	v.EventuallyWithT(func(c *assert.CollectT) {
		output, err := agentHost.Execute(cmd)
		assert.NoError(c, err)
		assert.Contains(c, output, "windows_certificate")
		v.T().Logf("Agent status: %s", output)
	}, 5*time.Minute, 10*time.Second)

	cmdCheck := fmt.Sprintf(`&'%s' check windows_certificate`, agent)
	certStoreTag := `"certificate_store:CA"`
	serverTag := fmt.Sprintf(`"server:%s"`, certificateHost.HostOutput.Address)
	issuerOUTag := `"crl_issuer_OU:VeriSign Commercial Software Publishers CA"`
	output, err := agentHost.Execute(cmdCheck)
	v.Require().NoError(err)
	v.T().Logf("Check output: %s", output)

	// Assert that the check output returns the metric and service check
	v.Require().Contains(output, "windows_certificate.crl_days_remaining")
	v.Require().Contains(output, "windows_certificate.crl_expiration")

	// Assert that the check output returns the correct metric tags
	v.Require().Contains(output, certStoreTag)
	v.Require().Contains(output, serverTag)
	v.Require().Contains(output, issuerOUTag)

}

func RebootHost(host *components.RemoteHost) error {
	_, err := powershell.PsHost().Reboot().Execute(host)
	if err != nil {
		return err
	}
	return nil
}
