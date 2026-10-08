// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package domain

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	platformCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	rodcSiteName    = "RODC-Site"
	writableDCName  = "writable-dc"
	readOnlyDCName  = "read-only-dc"
	locatorWritable = 0x00000100

	// Allow approximately 15 minutes for each reboot and Active Directory readiness check.
	maxRODCRetryAttempts = 90
	rodcRetryDelay       = 10 * time.Second
)

//go:embed fixtures/discover-domain-controller.ps1
var discoverDomainControllerScript string

type rodcEnvironment struct {
	WritableController *components.RemoteHost
	ReadOnlyController *components.RemoteHost
}

type testRODCInstallSuite struct {
	windows.BaseAgentInstallerSuite[rodcEnvironment]
}

type locatorResult struct {
	DomainControllerName string
	Flags                uint32
	LocalIsReadOnly      bool
	LocalSite            string
	RemoteSite           string
}

func TestInstallsOnReadOnlyDomainController(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &testRODCInstallSuite{},
		e2e.WithSkipDeleteOnFailure(),
		e2e.WithPulumiProvisioner(rodcProvisioner(), nil))
}

func (suite *testRODCInstallSuite) TestInstallUsesLocalReadOnlyRole() {
	host := suite.Env().ReadOnlyController

	encodedScript := base64.StdEncoding.EncodeToString([]byte(discoverDomainControllerScript))
	resultJSON := host.MustExecute(fmt.Sprintf(
		"$script = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); & ([ScriptBlock]::Create($script)) -Domain '%s'",
		encodedScript,
		TestDomain,
	))
	var locator locatorResult
	suite.Require().NoError(json.Unmarshal([]byte(resultJSON), &locator))

	localName := strings.TrimSpace(host.MustExecute("hostname"))
	selectedName := strings.Split(strings.TrimPrefix(locator.DomainControllerName, `\\`), ".")[0]
	suite.Require().True(locator.LocalIsReadOnly, "the installation target must be an RODC")
	suite.Require().NotEqual(strings.ToLower(localName), strings.ToLower(selectedName),
		"DS_AVOID_SELF must observe the remote controller used to simulate the customer topology")
	suite.Require().NotZero(locator.Flags&locatorWritable,
		"the remote controller must be writable")
	suite.Require().NotEqual(locator.LocalSite, locator.RemoteSite,
		"the writable controller and local RODC must be in different sites")

	logPath := filepath.Join(suite.SessionOutputDir(), "rodc-install.log")
	_, err := suite.InstallAgent(host,
		windowsAgent.WithPackage(suite.AgentPackage),
		windowsAgent.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
		windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windowsAgent.WithZeroAPIKey(),
		windowsAgent.WithInstallLogFile(logPath))
	suite.Require().NoError(err, "should install the Agent on an RODC when locator discovery selects a remote writable controller")

	installLog, err := os.ReadFile(logPath)
	suite.Require().NoError(err)
	installLog, err = windowsCommon.ConvertUTF16ToUTF8(installLog)
	suite.Require().NoError(err)
	suite.True(strings.Contains(string(installLog), "Host is a Read-Only Domain controller"),
		"the installer log should report the local read-only controller role")

	testClient := suite.NewTestClientForHost(host)
	testClient.CheckAgentVersion(suite.T(), suite.AgentPackage.AgentVersion())
	platformCommon.CheckAgentBehaviour(suite.T(), testClient)
}

func rodcProvisioner() provisioners.PulumiEnvRunFunc[rodcEnvironment] {
	return func(ctx *pulumi.Context, env *rodcEnvironment) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		writableController, err := ec2.NewVM(awsEnv, writableDCName, ec2.WithOS(e2eos.WindowsServer2022E2E))
		if err != nil {
			return err
		}
		if err := writableController.Export(ctx, &env.WritableController.HostOutput); err != nil {
			return err
		}

		readOnlyController, err := ec2.NewVM(awsEnv, readOnlyDCName, ec2.WithOS(e2eos.WindowsServer2022E2E))
		if err != nil {
			return err
		}
		if err := readOnlyController.Export(ctx, &env.ReadOnlyController.HostOutput); err != nil {
			return err
		}

		_, writableResources, err := activedirectory.NewActiveDirectory(
			ctx,
			&awsEnv,
			writableController,
			activedirectory.WithDomainController(TestDomain, TestPassword),
			activedirectory.WithDomainUser(TestUser, TestPassword),
		)
		if err != nil {
			return err
		}

		siteCommand, err := writableController.OS.Runner().Command("configure-rodc-site", &command.Args{
			Create: pulumi.Sprintf(`
Import-Module ActiveDirectory
$site = Get-ADReplicationSite -Filter "Name -eq '%s'"
if (-not $site) {
    $site = New-ADReplicationSite -Name '%s' -PassThru
}
$siteLink = Get-ADReplicationSiteLink -Identity 'DEFAULTIPSITELINK'
$includedSites = @($siteLink.SitesIncluded | ForEach-Object { $_.ToString() })
if ($includedSites -notcontains $site.DistinguishedName) {
    Set-ADReplicationSiteLink -Identity $siteLink -SitesIncluded @{ Add = $site.DistinguishedName }
}
$subnetName = '%s/32'
if (-not (Get-ADReplicationSubnet -Filter "Name -eq '$subnetName'")) {
    New-ADReplicationSubnet -Name $subnetName -Site $site | Out-Null
}
`, rodcSiteName, rodcSiteName, readOnlyController.Address),
		}, pulumi.DependsOn(writableResources))
		if err != nil {
			return err
		}

		joinCommand, err := readOnlyController.OS.Runner().Command("join-domain", &command.Args{
			Create: pulumi.Sprintf(`
Get-NetAdapter | Where-Object Status -eq 'Up' | Set-DnsClientServerAddress -ServerAddresses '%s'
Install-WindowsFeature -Name AD-Domain-Services -IncludeManagementTools | Out-Null
$domainAdminPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
$credential = New-Object System.Management.Automation.PSCredential('Administrator@%s', $domainAdminPassword)
Add-Computer -DomainName '%s' -Credential $credential -ErrorAction Stop
(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc() | Set-Content C:\domain-join-preboot.txt -NoNewline
shutdown.exe /r /f /t 5
`, writableController.Address, writableController.Password, TestDomain, TestDomain),
		}, pulumi.DependsOn([]pulumi.Resource{siteCommand}))
		if err != nil {
			return err
		}

		joinHook, err := registerRODCRetryHook(ctx, "wait-for-domain-join")
		if err != nil {
			return err
		}
		joinReadyCommand, err := readOnlyController.OS.Runner().Command("wait-for-domain-join", &command.Args{
			Create: pulumi.String(`
$baseline = [long](Get-Content C:\domain-join-preboot.txt)
if ((Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc() -le $baseline) {
    throw 'host has not rebooted since joining the domain'
}
if (-not (Get-CimInstance Win32_ComputerSystem).PartOfDomain) {
    throw 'host has not joined the domain'
}
`),
		}, pulumi.DependsOn([]pulumi.Resource{joinCommand}), pulumi.ResourceHooks(&pulumi.ResourceHookBinding{
			OnError: []*pulumi.ErrorHook{joinHook},
		}))
		if err != nil {
			return err
		}

		promoteCommand, err := readOnlyController.OS.Runner().Command("promote-rodc", &command.Args{
			Create: pulumi.Sprintf(`
Import-Module ADDSDeployment
$domainAdminPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
$credential = New-Object System.Management.Automation.PSCredential('Administrator@%s', $domainAdminPassword)
$safeModePassword = ConvertTo-SecureString '%s' -AsPlainText -Force
$arguments = @{
    Credential                    = $credential
    DomainName                    = '%s'
    InstallDns                    = $true
    NoRebootOnCompletion          = $true
    ReadOnlyReplica               = $true
    SafeModeAdministratorPassword = $safeModePassword
    SiteName                      = '%s'
    Force                         = $true
}
Install-ADDSDomainController @arguments
(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc() | Set-Content C:\rodc-promotion-preboot.txt -NoNewline
shutdown.exe /r /f /t 5
`, writableController.Password, TestDomain, TestPassword, TestDomain, rodcSiteName),
		}, pulumi.DependsOn([]pulumi.Resource{joinReadyCommand}))
		if err != nil {
			return err
		}

		promotionHook, err := registerRODCRetryHook(ctx, "wait-for-rodc-promotion")
		if err != nil {
			return err
		}
		_, err = readOnlyController.OS.Runner().Command("wait-for-rodc-promotion", &command.Args{
			Create: pulumi.String(`
$baseline = [long](Get-Content C:\rodc-promotion-preboot.txt)
if ((Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc() -le $baseline) {
    throw 'host has not rebooted since RODC promotion'
}
$adws = Get-Service ADWS -ErrorAction SilentlyContinue
if (-not ($adws -and $adws.Status -eq 'Running')) {
    throw 'ADWS is not running'
}
Import-Module ActiveDirectory
$controller = Get-ADDomainController -Identity $env:COMPUTERNAME -ErrorAction Stop
if (-not $controller.IsReadOnly) {
    throw 'local controller is not read-only'
}
`),
		}, pulumi.DependsOn([]pulumi.Resource{promoteCommand}), pulumi.ResourceHooks(&pulumi.ResourceHookBinding{
			OnError: []*pulumi.ErrorHook{promotionHook},
		}))
		return err
	}
}

func registerRODCRetryHook(ctx *pulumi.Context, name string) (*pulumi.ErrorHook, error) {
	return ctx.RegisterErrorHook(name, func(args *pulumi.ErrorHookArgs) (bool, error) {
		attempt := len(args.Errors)
		if attempt > 0 && strings.Contains(args.Errors[0], "failed attempts: dial") {
			return false, nil
		}
		if attempt >= maxRODCRetryAttempts {
			return false, nil
		}
		time.Sleep(rodcRetryDelay)
		return true, nil
	})
}
