// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main is the Pulumi entrypoint for the netpath-vm-windows scenario:
// a long-lived AWS EC2 Windows Server 2022 VM running the Datadog Agent with
// Cloud Network Monitoring (NPM) and Network Path dynamic tests enabled,
// optionally with the CrowdStrike Falcon sensor, plus a periodic outbound
// TCP/UDP workload so dynamic paths have live flows to discover.
//
// Sibling to the Linux netpath-vm scenario. One Pulumi stack per Datadog
// environment. The program is self-contained — it does not depend on the
// e2e-framework, so it works in any AWS account where the caller has EC2
// + IAM permissions. Access to the VM is via AWS Systems Manager Session
// Manager (no RDP, no inbound rules).
package main

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

//go:embed assets/targets.txt
var defaultTargets string

//go:embed assets/conn-gen.ps1
var connGenScript string

//go:embed assets/network_path.yaml
var defaultNetworkPathIntegration string

const (
	configNamespace      = "netpath"
	defaultSite          = "datadoghq.com"
	defaultInstanceType  = "t3.xlarge"
	defaultName          = "netpath-vm-windows"
	defaultRootVolumeGiB = 60
	defaultWorkers       = 4

	// Datadog Installer bootstrap binary. Reads DD_API_KEY / DD_SITE /
	// DD_TAGS / DD_AGENT_VERSION (when supported) from the environment.
	datadogInstallerURL = "https://install.datadoghq.com/datadog-installer-x86_64.exe"

	// SSM public parameter for the latest Windows Server 2022 English Full Base AMI.
	windows2022AMIParameter = "/aws/service/ami-windows-latest/Windows_Server-2022-English-Full-Base"

	// AWS-managed policy granting just enough perms for SSM Session Manager
	// to register the instance and accept sessions.
	ssmManagedInstanceCorePolicy = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"

	ec2AssumeRolePolicy = `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Service": "ec2.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }]
}`
)

func main() {
	pulumi.Run(run)
}

func run(ctx *pulumi.Context) error {
	cfg := config.New(ctx, configNamespace)

	apiKey := cfg.RequireSecret("apiKey")
	// Both CrowdStrike inputs are optional. When either is absent the
	// Falcon sensor install step is skipped entirely.
	csMsiURL := cfg.GetSecret("crowdstrikeMsiUrl")
	csCID := cfg.GetSecret("crowdstrikeCid")

	site := getOr(cfg, "site", defaultSite)
	name := getOr(cfg, "name", defaultName)
	instanceType := getOr(cfg, "instanceType", defaultInstanceType)
	targets := getOr(cfg, "targets", defaultTargets)
	tagsCSV := strings.Join(splitAndTrim(cfg.Get("tags"), ","), ",")
	agentVersion := strings.TrimSpace(cfg.Get("agentVersion"))
	rootVolumeGiB := getIntOr(cfg, "rootVolumeSizeGiB", defaultRootVolumeGiB)
	workers := getIntOr(cfg, "workers", defaultWorkers)

	scheduledIntegration := ""
	if getBoolOr(cfg, "enableScheduled", true) {
		scheduledIntegration = getOr(cfg, "scheduledConfig", defaultNetworkPathIntegration)
	}

	defaultVpc, err := ec2.LookupVpc(ctx, &ec2.LookupVpcArgs{
		Default: pulumi.BoolRef(true),
	}, nil)
	if err != nil {
		return fmt.Errorf("lookup default VPC: %w", err)
	}

	subnets, err := ec2.GetSubnets(ctx, &ec2.GetSubnetsArgs{
		Filters: []ec2.GetSubnetsFilter{
			{Name: "vpc-id", Values: []string{defaultVpc.Id}},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("list subnets in default VPC %s: %w", defaultVpc.Id, err)
	}
	if len(subnets.Ids) == 0 {
		return fmt.Errorf("no subnets found in default VPC %s", defaultVpc.Id)
	}

	ami, err := ssm.LookupParameter(ctx, &ssm.LookupParameterArgs{
		Name: windows2022AMIParameter,
	}, nil)
	if err != nil {
		return fmt.Errorf("lookup Windows Server 2022 AMI: %w", err)
	}

	role, err := iam.NewRole(ctx, name+"-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(ec2AssumeRolePolicy),
		Description:      pulumi.Sprintf("EC2 instance role for %s (SSM Session Manager access)", name),
	})
	if err != nil {
		return err
	}

	_, err = iam.NewRolePolicyAttachment(ctx, name+"-ssm", &iam.RolePolicyAttachmentArgs{
		Role:      role.Name,
		PolicyArn: pulumi.String(ssmManagedInstanceCorePolicy),
	})
	if err != nil {
		return err
	}

	instanceProfile, err := iam.NewInstanceProfile(ctx, name+"-profile", &iam.InstanceProfileArgs{
		Role: role.Name,
	})
	if err != nil {
		return err
	}

	// Egress-only security group: SSM Session Manager doesn't need any
	// inbound rules — the SSM agent on the instance opens an outbound
	// connection to the SSM service.
	sg, err := ec2.NewSecurityGroup(ctx, name+"-sg", &ec2.SecurityGroupArgs{
		VpcId:       pulumi.String(defaultVpc.Id),
		Description: pulumi.Sprintf("Egress-only SG for %s (SSM access)", name),
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				Protocol:    pulumi.String("-1"),
				FromPort:    pulumi.Int(0),
				ToPort:      pulumi.Int(0),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Description: pulumi.String("All egress"),
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-sg")},
	})
	if err != nil {
		return err
	}

	// pulumi.All resolves the (potentially secret) inputs together so we
	// can splice them into the user-data PowerShell script.
	userData := pulumi.All(apiKey, csMsiURL, csCID).ApplyT(func(args []interface{}) string {
		key, _ := args[0].(string)
		csURL, _ := args[1].(string)
		cid, _ := args[2].(string)
		return buildUserData(userDataInputs{
			apiKey:               key,
			site:                 site,
			tagsCSV:              tagsCSV,
			agentVersion:         agentVersion,
			crowdstrikeMSIURL:    csURL,
			crowdstrikeCID:       cid,
			targets:              targets,
			scheduledIntegration: scheduledIntegration,
			workers:              workers,
		})
	}).(pulumi.StringOutput)

	instance, err := ec2.NewInstance(ctx, name, &ec2.InstanceArgs{
		Ami:                      pulumi.String(ami.Value),
		InstanceType:             pulumi.String(instanceType),
		IamInstanceProfile:       instanceProfile.Name,
		SubnetId:                 pulumi.String(subnets.Ids[0]),
		VpcSecurityGroupIds:      pulumi.StringArray{sg.ID()},
		AssociatePublicIpAddress: pulumi.Bool(true),
		UserData:                 userData,
		UserDataReplaceOnChange:  pulumi.Bool(true),
		RootBlockDevice: &ec2.InstanceRootBlockDeviceArgs{
			VolumeSize: pulumi.Int(rootVolumeGiB),
			VolumeType: pulumi.String("gp3"),
		},
		Tags: pulumi.StringMap{
			"Name":    pulumi.String(name),
			"purpose": pulumi.String("netpath-demo-windows"),
		},
	})
	if err != nil {
		return err
	}

	ctx.Export("vmInstanceId", instance.ID())
	ctx.Export("vmPublicIp", instance.PublicIp)
	ctx.Export("ssmCommand", pulumi.Sprintf("aws ssm start-session --target %s", instance.ID()))

	return nil
}

type userDataInputs struct {
	apiKey               string
	site                 string
	tagsCSV              string
	agentVersion         string // empty = latest (installer default)
	crowdstrikeMSIURL    string
	crowdstrikeCID       string
	targets              string
	scheduledIntegration string
	workers              int
}

// buildUserData composes the PowerShell script EC2 runs on first boot.
// All string inputs are base64-encoded before being spliced into the
// script so we never have to worry about PowerShell quoting rules.
func buildUserData(in userDataInputs) string {
	datadogYAML := buildDatadogYAML(in.apiKey, in.site, in.tagsCSV, in.workers)
	systemProbeYAML := "network_config:\n  enabled: true\ntraceroute:\n  enabled: true\n"

	return fmt.Sprintf(`<powershell>
$ErrorActionPreference = 'Stop'
Start-Transcript -Path 'C:\Windows\Temp\bootstrap.log' -Append

function FromB64([string]$s) {
    if (-not $s) { return '' }
    return [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($s))
}

$apiKey           = FromB64 '%s'
$site             = FromB64 '%s'
$tagsCSV          = FromB64 '%s'
$agentVersion     = FromB64 '%s'
$csMsiUrl         = FromB64 '%s'
$csCid            = FromB64 '%s'

$targets          = FromB64 '%s'
$connGenScript    = FromB64 '%s'
$datadogYaml      = FromB64 '%s'
$systemProbeYaml  = FromB64 '%s'
$networkPathYaml  = FromB64 '%s'

$ddProgramData = 'C:\ProgramData\Datadog'
New-Item -ItemType Directory -Force -Path $ddProgramData | Out-Null
[IO.File]::WriteAllText((Join-Path $ddProgramData 'conn-gen-targets.txt'), $targets)
[IO.File]::WriteAllText((Join-Path $ddProgramData 'conn-gen.ps1'),         $connGenScript)

# --- CrowdStrike Falcon sensor (optional) ---
# All paths under C:\Windows\Temp deliberately — no spaces, no quoting needed.
if ($csMsiUrl -and $csCid) {
    $csInstaller = 'C:\Windows\Temp\FalconSensor_Windows.msi'
    Invoke-WebRequest -Uri $csMsiUrl -OutFile $csInstaller -UseBasicParsing
    $csLog = 'C:\Windows\Temp\falcon-install.log'
    $proc  = Start-Process -FilePath msiexec.exe -Wait -PassThru -ArgumentList @(
        '/quiet','/norestart','/i', $csInstaller, "CID=$csCid", '/L*v', $csLog
    )
    if ($proc.ExitCode -ne 0) {
        throw "CrowdStrike install failed (exit $($proc.ExitCode)); see $csLog"
    }
}

# --- Datadog Agent (via Datadog Installer bootstrap binary) ---
# Env vars drive the install. DD_AGENT_VERSION may or may not be honored
# depending on installer build; harmless when not — defaults to latest.
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
$env:DD_API_KEY = $apiKey
$env:DD_SITE    = $site
if ($tagsCSV)      { $env:DD_TAGS          = $tagsCSV }
if ($agentVersion) { $env:DD_AGENT_VERSION = $agentVersion }

$ddInstaller = 'C:\Windows\SystemTemp\datadog-installer-x86_64.exe'
(New-Object System.Net.WebClient).DownloadFile('%s', $ddInstaller)
& $ddInstaller
if ($LASTEXITCODE -ne 0) {
    throw "Datadog installer failed with exit code $LASTEXITCODE"
}

# --- Overwrite agent configs with our network-path-aware versions ---
[IO.File]::WriteAllText((Join-Path $ddProgramData 'datadog.yaml'),       $datadogYaml)
[IO.File]::WriteAllText((Join-Path $ddProgramData 'system-probe.yaml'),  $systemProbeYaml)

if ($networkPathYaml) {
    $npDir = Join-Path $ddProgramData 'conf.d\network_path.d'
    New-Item -ItemType Directory -Force -Path $npDir | Out-Null
    [IO.File]::WriteAllText((Join-Path $npDir 'conf.yaml'), $networkPathYaml)
}

# --- Restart Datadog services to pick up the new configs ---
Restart-Service -Name 'datadogagent' -Force
Start-Sleep -Seconds 5
$sp = Get-Service -Name 'datadog-system-probe' -ErrorAction SilentlyContinue
if ($sp) { Restart-Service -Name 'datadog-system-probe' -Force }

# --- conn-gen scheduled task: fire every minute, indefinitely ---
$action    = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument '-NoProfile -ExecutionPolicy Bypass -File C:\ProgramData\Datadog\conn-gen.ps1'
$trigger   = New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Minutes 1) -RepetitionDuration (New-TimeSpan -Days 9999)
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings  = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName 'conn-gen' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null

Stop-Transcript
</powershell>
`,
		b64(in.apiKey),
		b64(in.site),
		b64(in.tagsCSV),
		b64(in.agentVersion),
		b64(in.crowdstrikeMSIURL),
		b64(in.crowdstrikeCID),
		b64(in.targets),
		b64(connGenScript),
		b64(datadogYAML),
		b64(systemProbeYAML),
		b64(in.scheduledIntegration),
		datadogInstallerURL,
	)
}

func buildDatadogYAML(apiKey, site, tagsCSV string, workers int) string {
	if workers < 1 {
		workers = defaultWorkers
	}
	var b strings.Builder
	fmt.Fprintf(&b, "api_key: %s\nsite: %s\n", apiKey, site)
	if tagsCSV != "" {
		b.WriteString("tags:\n")
		for _, t := range strings.Split(tagsCSV, ",") {
			if t = strings.TrimSpace(t); t != "" {
				fmt.Fprintf(&b, "  - %s\n", t)
			}
		}
	}
	// network_path drives both the scheduled (integration) and dynamic
	// (connections_monitoring) path tests. synthetics.collector.enabled
	// is what lets the org's synthetic Network Path tests target this host.
	//
	// telemetry / internal_profiling / process_config are on by default —
	// this scenario is built for perf testing the agent under synthetic
	// Network Path load, and those three blocks expose the internal
	// scheduler metrics, CPU/heap profiles, and per-process CPU/RAM
	// time series needed to characterize the resource cost.
	fmt.Fprintf(&b, `network_path:
  connections_monitoring:
    enabled: true
  collector:
    workers: %d
    pathtest_interval: 5m
    pathtest_ttl: 70m
    timeout: 1000
synthetics:
  collector:
    enabled: true
telemetry:
  enabled: true
  checks: "*"
internal_profiling:
  enabled: true
process_config:
  process_collection:
    enabled: true
`, workers)
	return b.String()
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func getOr(cfg *config.Config, key, fallback string) string {
	if v := cfg.Get(key); v != "" {
		return v
	}
	return fallback
}

func getIntOr(cfg *config.Config, key string, fallback int) int {
	v := cfg.Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getBoolOr(cfg *config.Config, key string, fallback bool) bool {
	v := cfg.Get(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
