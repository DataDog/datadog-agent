// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package activedirectory

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-time/sdk/go/time"
)

// Configuration is an object representing the desired Active Directory configuration.
type Configuration struct {
	JoinDomainParams              *JoinDomainConfiguration
	DomainControllerConfiguration *DomainControllerConfiguration
	DomainUsers                   []DomainUser
	ResourceOptions               []pulumi.ResourceOption
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithPulumiResourceOptions sets some pulumi resource option, like which resource
// to depend on.
func WithPulumiResourceOptions(resources ...pulumi.ResourceOption) Option {
	return func(p *Configuration) error {
		p.ResourceOptions = resources
		return nil
	}
}

// JoinDomainConfiguration list the options required for a machine to join an Active Directory domain.
type JoinDomainConfiguration struct {
	DomainName              string
	DomainAdminUser         string
	DomainAdminUserPassword string
}

// WithDomain joins a machine to a domain. The machine can then be promoted to a domain controller or remain
// a domain client.
func WithDomain(domainFqdn, domainAdmin, domainAdminPassword string) Option {
	return func(p *Configuration) error {
		p.JoinDomainParams = &JoinDomainConfiguration{
			DomainName:              domainFqdn,
			DomainAdminUser:         domainAdmin,
			DomainAdminUserPassword: domainAdminPassword,
		}
		return nil
	}
}

func (adCtx *activeDirectoryContext) joinActiveDirectoryDomain(params *JoinDomainConfiguration) error {
	var joinCmd command.Command
	joinCmd, err := adCtx.comp.host.OS.Runner().Command(adCtx.comp.namer.ResourceName("join-domain"), &command.Args{
		Create: pulumi.Sprintf(`
Add-Computer -DomainName %s -Credential (New-Object System.Management.Automation.PSCredential -ArgumentList %s, %s)
`, params.DomainName, params.DomainAdminUser, params.DomainAdminUserPassword),
	}, pulumi.Parent(adCtx.comp))
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, joinCmd)

	waitForRebootAfterJoiningCmd, err := time.NewSleep(adCtx.pulumiContext, adCtx.comp.namer.ResourceName("wait-for-host-to-reboot-after-joining-domain"), &time.SleepArgs{
		CreateDuration: pulumi.String("30s"),
	},
		pulumi.Provider(adCtx.timeProvider),
		pulumi.DependsOn(adCtx.createdResources)) // Depend on all the previously created resources
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, waitForRebootAfterJoiningCmd)
	return nil
}

// DomainControllerConfiguration represents the Active Directory configuration (domain name, password, users etc...)
type DomainControllerConfiguration struct {
	DomainName     string
	DomainPassword string
}

// WithDomainController promotes the machine to be a domain controller.
func WithDomainController(domainFqdn, adminPassword string) func(*Configuration) error {
	return func(p *Configuration) error {
		p.DomainControllerConfiguration = &DomainControllerConfiguration{
			DomainName:     domainFqdn,
			DomainPassword: adminPassword,
		}
		return nil
	}
}

// Windows Server 2025 requires functional level of 7 (WinThreshold). To achieve better consistency, we use number representation.
// https://learn.microsoft.com/en-us/powershell/module/addsdeployment/install-addsforest?view=windowsserver2022-ps&viewFallbackFrom=win10-ps
func (adCtx *activeDirectoryContext) installDomainController(params *DomainControllerConfiguration) error {
	var installCmd command.Command
	installCmd, err := adCtx.comp.host.OS.Runner().Command(adCtx.comp.namer.ResourceName("install-forest"), &command.Args{
		Create: pulumi.Sprintf(`
Add-WindowsFeature -name ad-domain-services -IncludeManagementTools;
Import-Module ADDSDeployment;
try {
	Get-ADDomainController
} catch {
	$HashArguments = @{
		CreateDNSDelegation           = $false
		ForestMode                    = "7"
		DomainMode                    = "7"
		DomainName                    = "%s"
		SafeModeAdministratorPassword = (ConvertTo-SecureString %s -AsPlainText -Force)
		NoRebootOnCompletion          = $true
		Force                         = $true
	}; Install-ADDSForest @HashArguments
	# Record the pre-reboot boot time so ensure-adws-started can deterministically confirm the reboot
	# completed (a newer boot time) instead of racing a fixed sleep.
	(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc() | Set-Content -Path C:\dcpromo-preboot.txt -NoNewline
	# Issue the single, controlled reboot 5 seconds out so this command returns success before SSH drops.
	shutdown.exe /r /f /t 5
}
`, params.DomainName, params.DomainPassword),
	}, pulumi.Parent(adCtx.comp), pulumi.DependsOn(adCtx.createdResources))
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, installCmd)

	waitForRebootCmd, err := time.NewSleep(adCtx.pulumiContext, adCtx.comp.namer.ResourceName("wait-for-host-to-reboot"), &time.SleepArgs{
		CreateDuration: pulumi.String("90s"),
	},
		pulumi.Provider(adCtx.timeProvider),
		pulumi.DependsOn(adCtx.createdResources)) // Depend on all the previously created resources
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, waitForRebootCmd)

	// Wait for the DC to be ready for AD operations. Three checks, in order:
	//   1. ADWS service is Running.
	//   2. Get-ADDomain succeeds.
	//   3. RID allocation works — probe by creating and deleting a throwaway user.
	// The third check is needed because Get-ADDomain returns while the DC is still finishing post-boot
	// stabilization (quota-tracking table rebuild, FSMO role assumption). Writes that need a fresh RID
	// block (New-ADUser, New-ADServiceAccount, etc.) fail with "directory service was unable to allocate
	// a relative identifier" (ActiveDirectoryServer:8208) until that stabilization completes.
	// See WINA-2095.
	//
	// ensure-adws-started runs only after wait-for-host-to-reboot, and first confirms the host has
	// actually rebooted since promotion (a boot time newer than the recorded baseline) before probing
	// ADWS/AD — so it never races the reboot. See WINA-2876.
	ensureAdwsStartedCmd, err := adCtx.comp.host.OS.Runner().Command(adCtx.comp.namer.ResourceName("ensure-adws-started"), &command.Args{
		Create: pulumi.String(`
# Deterministically wait for the controlled post-promotion reboot to complete. install-forest records
# the pre-reboot boot time; skip when no promotion happened this run (file absent = already a DC).
if (Test-Path C:\dcpromo-preboot.txt) {
    $baseline = [long](Get-Content C:\dcpromo-preboot.txt)
    $deadline = [DateTime]::Now.AddMinutes(15)
    while ([DateTime]::Now -lt $deadline) {
        if ((Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc() -gt $baseline) { break }
        Start-Sleep -Seconds 10
    }
    if ([DateTime]::Now -ge $deadline) { throw "host did not reboot after promotion" }
}

# Wait for ADWS to be Running, nudging it if necessary. A freshly-promoted DC can take well over a
# minute to bring ADWS up, so poll instead of a single 60s WaitForStatus.
$deadline = [DateTime]::Now.AddMinutes(5)
while ([DateTime]::Now -lt $deadline) {
    $svc = Get-Service ADWS -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -eq 'Running') { break }
    try { Start-Service ADWS -ErrorAction Stop } catch {}
    Start-Sleep -Seconds 5
}
if ((Get-Service ADWS -ErrorAction SilentlyContinue).Status -ne 'Running') {
    throw "ADWS did not reach Running state"
}
$timeout = [DateTime]::Now.AddMinutes(5)
while ([DateTime]::Now -lt $timeout) {
    try {
        Get-ADDomain
        break
    } catch {
        Start-Sleep -Seconds 5
    }
}
if ([DateTime]::Now -ge $timeout) {
    throw "Get-ADDomain timed out"
}

# Generate a random password (cryptographically secure) that satisfies default AD password policy.
$rngBytes = New-Object byte[] 24
[System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($rngBytes)
$probePassword = [Convert]::ToBase64String($rngBytes) + "Aa1!"

# Probe RID allocation by creating and deleting a throwaway user.
$timeout = [DateTime]::Now.AddMinutes(10)
while ([DateTime]::Now -lt $timeout) {
    $probeName = "rid-probe-$([guid]::NewGuid().ToString('N').Substring(0,8))"
    try {
        New-ADUser -Name $probeName -AccountPassword (ConvertTo-SecureString $probePassword -AsPlainText -Force) -Enabled $false -ErrorAction Stop
        Remove-ADUser -Identity $probeName -Confirm:$false -ErrorAction Stop
        break
    } catch {
        # Best-effort cleanup in case New-ADUser succeeded after a transient hiccup but Remove-ADUser didn't run.
        try { Remove-ADUser -Identity $probeName -Confirm:$false -ErrorAction Stop } catch {}
        Start-Sleep -Seconds 5
    }
}
if ([DateTime]::Now -ge $timeout) {
    throw "RID allocation probe timed out — DC not ready to issue RID pools"
}
`),
	}, utils.PulumiDependsOn(waitForRebootCmd))
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, ensureAdwsStartedCmd)
	return nil
}

// DomainUser represents an Active Directory user
type DomainUser struct {
	Username string
	Password string
}

// WithDomainUser adds a user in Active Directory.
// Note: We don't need to be a Domain Controller to create new user in AD but we need
// the necessary rights to modify the AD.
func WithDomainUser(username, password string) func(params *Configuration) error {
	return func(p *Configuration) error {
		p.DomainUsers = append(p.DomainUsers, DomainUser{
			Username: username,
			Password: password,
		})
		return nil
	}
}
