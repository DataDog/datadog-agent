// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package activedirectory

import (
	"strings"
	stdtime "time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-time/sdk/go/time"
)

const (
	// ~15 minute budget for wait-for-reboot-since-promotion's onError retries.
	maxRebootPollAttempts = 90
	rebootPollRetryDelay  = 10 * stdtime.Second

	// ~5 minute budget for ensure-adws-running's onError retries.
	maxADWSPollAttempts = 60
	adwsPollRetryDelay  = 5 * stdtime.Second

	// ~5 minute budget for ensure-ad-domain-ready's onError retries.
	maxADDomainPollAttempts = 60
	adDomainPollRetryDelay  = 5 * stdtime.Second

	// ~10 minute budget for ensure-rid-allocation-ready's onError retries. Pulumi hard-caps
	// onError hook retries at 100, so this uses a longer delay (rather than more attempts) to
	// still cover the same budget the previous inline loop used.
	maxRIDPollAttempts = 100
	ridPollRetryDelay  = 6 * stdtime.Second
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

// registerPollRetryHook registers an onError hook for a single-shot, idempotent check command. It
// retries unconditionally up to maxAttempts (with a fixed delay between attempts), except when the
// error indicates SSH dial exhaustion -- that means the host is genuinely unreachable, not just
// "not ready yet", and retrying at this level won't help.
//
// Pulumi already reports each hook-triggered retry itself ("retrying create due to on-error hook
// request (N/100)") plus the triggering error against the resource, so this doesn't log separately.
func (adCtx *activeDirectoryContext) registerPollRetryHook(name string, maxAttempts int, delay stdtime.Duration) (*pulumi.ErrorHook, error) {
	return adCtx.pulumiContext.RegisterErrorHook(adCtx.comp.namer.ResourceName(name+"-retry"), func(args *pulumi.ErrorHookArgs) (bool, error) {
		attempt := len(args.Errors)
		if attempt > 0 && strings.Contains(args.Errors[0], "failed attempts: dial") {
			return false, nil
		}
		if attempt >= maxAttempts {
			return false, nil
		}
		stdtime.Sleep(delay)
		return true, nil
	})
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
	# Record the pre-reboot boot time so wait-for-reboot-since-promotion can deterministically confirm
	# the reboot completed (a newer boot time) instead of racing a fixed sleep.
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

	// The reboot always takes at least ~30s, so a bare check would guarantee at least one failed
	// attempt (and a retry round-trip) every single run. This sleep is just a cheap pre-buffer to skip
	// that -- wait-for-reboot-since-promotion below still does the real, deterministic wait.
	waitForRebootCmd, err := time.NewSleep(adCtx.pulumiContext, adCtx.comp.namer.ResourceName("wait-for-host-to-reboot"), &time.SleepArgs{
		CreateDuration: pulumi.String("30s"),
	},
		pulumi.Provider(adCtx.timeProvider),
		pulumi.DependsOn(adCtx.createdResources)) // Depend on all the previously created resources
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, waitForRebootCmd)

	// Wait for the DC to be ready for AD operations. Checks, in order:
	//   1. Host has rebooted since promotion.
	//   2. ADWS service is Running.
	//   3. Get-ADDomain succeeds.
	//   4. RID allocation works — probe by creating and deleting a throwaway user.
	// Each as a separate resource with a pulumi onError hook to retry the check on failure,
	// granting us error message visibility at each step, which we don't get from a single resource
	// with inline retry loops.
	// See WINA-2095, WINA-2876.

	rebootHook, err := adCtx.registerPollRetryHook("wait-for-reboot-since-promotion", maxRebootPollAttempts, rebootPollRetryDelay)
	if err != nil {
		return err
	}
	waitForRebootSincePromotionCmd, err := adCtx.comp.host.OS.Runner().Command(adCtx.comp.namer.ResourceName("wait-for-reboot-since-promotion"), &command.Args{
		Create: pulumi.String(`
# install-forest records the pre-reboot boot time; skip when no promotion happened this run
# (file absent = already a DC). A newer boot time than the baseline means the reboot completed.
if (Test-Path C:\dcpromo-preboot.txt) {
    $baseline = [long](Get-Content C:\dcpromo-preboot.txt)
    if ((Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc() -le $baseline) {
        throw "host has not rebooted since promotion yet"
    }
}
`),
	}, utils.PulumiDependsOn(waitForRebootCmd), pulumi.ResourceHooks(&pulumi.ResourceHookBinding{
		OnError: []*pulumi.ErrorHook{rebootHook},
	}))
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, waitForRebootSincePromotionCmd)

	adwsHook, err := adCtx.registerPollRetryHook("ensure-adws-running", maxADWSPollAttempts, adwsPollRetryDelay)
	if err != nil {
		return err
	}
	ensureAdwsRunningCmd, err := adCtx.comp.host.OS.Runner().Command(adCtx.comp.namer.ResourceName("ensure-adws-running"), &command.Args{
		Create: pulumi.String(`
# A freshly-promoted DC can take well over a minute to bring ADWS up.
$svc = Get-Service ADWS -ErrorAction SilentlyContinue
if (-not ($svc -and $svc.Status -eq 'Running')) {
    try { Start-Service ADWS -ErrorAction Stop } catch {}
    throw "ADWS is not Running yet"
}
`),
	}, utils.PulumiDependsOn(waitForRebootSincePromotionCmd), pulumi.ResourceHooks(&pulumi.ResourceHookBinding{
		OnError: []*pulumi.ErrorHook{adwsHook},
	}))
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, ensureAdwsRunningCmd)

	adDomainHook, err := adCtx.registerPollRetryHook("ensure-ad-domain-ready", maxADDomainPollAttempts, adDomainPollRetryDelay)
	if err != nil {
		return err
	}
	ensureAdDomainReadyCmd, err := adCtx.comp.host.OS.Runner().Command(adCtx.comp.namer.ResourceName("ensure-ad-domain-ready"), &command.Args{
		Create: pulumi.String(`
try {
    Get-ADDomain | Out-Null
} catch {
    throw "Get-ADDomain failed: $($_.Exception.Message)"
}
`),
	}, utils.PulumiDependsOn(ensureAdwsRunningCmd), pulumi.ResourceHooks(&pulumi.ResourceHookBinding{
		OnError: []*pulumi.ErrorHook{adDomainHook},
	}))
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, ensureAdDomainReadyCmd)

	ridHook, err := adCtx.registerPollRetryHook("ensure-rid-allocation-ready", maxRIDPollAttempts, ridPollRetryDelay)
	if err != nil {
		return err
	}
	ensureRIDAllocationReadyCmd, err := adCtx.comp.host.OS.Runner().Command(adCtx.comp.namer.ResourceName("ensure-rid-allocation-ready"), &command.Args{
		Create: pulumi.String(`
# Generate a random password (cryptographically secure) that satisfies default AD password policy.
$rngBytes = New-Object byte[] 24
[System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($rngBytes)
$probePassword = [Convert]::ToBase64String($rngBytes) + "Aa1!"
$probeName = "rid-probe-$([guid]::NewGuid().ToString('N').Substring(0,8))"

# Probe RID allocation by creating and deleting a throwaway user.
try {
    New-ADUser -Name $probeName -AccountPassword (ConvertTo-SecureString $probePassword -AsPlainText -Force) -Enabled $false -ErrorAction Stop
    Remove-ADUser -Identity $probeName -Confirm:$false -ErrorAction Stop
} catch {
    # Best-effort cleanup in case New-ADUser succeeded after a transient hiccup but Remove-ADUser didn't run.
    try { Remove-ADUser -Identity $probeName -Confirm:$false -ErrorAction Stop } catch {}
    throw "RID allocation probe failed: $($_.Exception.Message)"
}
`),
	}, utils.PulumiDependsOn(ensureAdDomainReadyCmd), pulumi.ResourceHooks(&pulumi.ResourceHookBinding{
		OnError: []*pulumi.ErrorHook{ridHook},
	}))
	if err != nil {
		return err
	}
	adCtx.createdResources = append(adCtx.createdResources, ensureRIDAllocationReadyCmd)
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
