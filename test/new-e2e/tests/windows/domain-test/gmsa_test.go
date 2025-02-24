// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"

	"github.com/DataDog/test-infra-definitions/components/activedirectory"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type dcWithClientEnv struct {
	DC              *components.RemoteHost
	Client          *components.RemoteHost
	ActiveDirectory *components.RemoteActiveDirectory
}

type gmsaSuite struct {
	windows.BaseAgentInstallerSuite[dcWithClientEnv]
}

func TestGMSAEnv(t *testing.T) {
	e2e.Run(t, &gmsaSuite{}, e2e.WithPulumiProvisioner(dcWithClientEnvProvisioner(), nil))
}

func (s *gmsaSuite) SetupSuite() {
	s.BaseAgentInstallerSuite.SetupSuite()

	var out string
	out = s.Env().DC.MustExecute("ipconfig")
	s.T().Log(out)
	out = s.Env().Client.MustExecute("ipconfig")
	s.T().Log(out)

	// Assume the domain controller is setup/ready via pulumi provisioner

	// Join client to domain if needed
	domain, err := windowsCommon.GetJoinedDomain(s.Env().Client)
	s.Require().NoError(err)
	if domain == "WORKGROUP" {
		// set domain controller IP as DNS server for Client
		err := windowsCommon.SetDNSServerForConnectionInterface(s.Env().Client, []string{s.Env().DC.Address})
		s.Require().NoError(err, "failed to set DNS server for client")
		// join the domain
		cmd := fmt.Sprintf(`$pwd = ConvertTo-SecureString "%s" -AsPlainText -Force; Add-Computer -DomainName %s -Credential ([System.Management.Automation.PSCredential]::New("%s\%s", $pwd))`, s.Env().DC.Password, TestDomain, TestDomain, "Administrator")
		out, err := s.Env().Client.Execute(cmd)
		s.Require().NoError(err, "failed to join the domain: %s", out)
		// restart
		err = windowsCommon.RebootAndWaitWithBackoff(s.Env().Client, windowsCommon.DefaultRebootBackoff())
		s.Require().NoError(err, "failed to restart the computer: %s", out)
		// verify domain
		newdomain, err := windowsCommon.GetJoinedDomain(s.Env().Client)
		s.Require().NoError(err)
		s.Require().NotEqual(domain, newdomain, "failed to join the domain")
	}

	// Create ddagentuser account
	cmd := fmt.Sprintf(`$pwd = ConvertTo-SecureString "%s" -AsPlainText -Force; New-ADUser -Name "%s" -AccountPassword $pwd -PasswordNeverExpires 1`, TestPassword, TestUser)
	out, err = s.Env().DC.Execute(cmd)
	if !strings.Contains(err.Error(), "The specified account already exists") {
		s.Require().NoError(err, "failed to create user: %s", out)
	}

}

func (s *gmsaSuite) TestStuff() {
	client := s.Env().Client
	_, err := s.InstallAgent(client,
		windowsAgent.WithPackage(s.AgentPackage),
		windowsAgent.WithAgentUser(TestUser),
		windowsAgent.WithAgentUserPassword(TestPassword),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "TC-INS-DC-006_install.log")))

	s.Require().NoError(err, "should succeed to install Agent on a Domain Client with a valid domain account & password")

}

func dcWithClientEnvProvisioner() provisioners.PulumiEnvRunFunc[dcWithClientEnv] {
	return func(ctx *pulumi.Context, env *dcWithClientEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// dc
		dc, err := ec2.NewVM(awsEnv, "dc", ec2.WithOS(os.WindowsDefault))
		if err != nil {
			return err
		}
		dc.Export(ctx, &env.DC.HostOutput)
		_, err = defender.NewDefender(awsEnv.CommonEnvironment, dc, defender.WithDefenderDisabled())
		if err != nil {
			return err
		}
		// domain controller setup
		activeDirectoryComp, _, err := activedirectory.NewActiveDirectory(ctx, &awsEnv, dc,
			activedirectory.WithDomainController(TestDomain, TestPassword),
			activedirectory.WithDomainUser(TestUser, TestPassword),
		)
		if err != nil {
			return err
		}
		err = activeDirectoryComp.Export(ctx, &env.ActiveDirectory.Output)
		if err != nil {
			return err
		}

		// client
		client, err := ec2.NewVM(awsEnv, "client", ec2.WithOS(os.WindowsDefault))
		if err != nil {
			return err
		}
		client.Export(ctx, &env.Client.HostOutput)
		_, err = defender.NewDefender(awsEnv.CommonEnvironment, client, defender.WithDefenderDisabled())
		if err != nil {
			return err
		}

		return nil
	}
}
