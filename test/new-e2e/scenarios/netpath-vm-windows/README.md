# netpath-vm-windows

Long-lived AWS EC2 **Windows Server 2022** VM running the Datadog Agent with
**Cloud Network Monitoring** (NPM), **Network Path** dynamic + scheduled tests,
and **synthetic Network Path** support, plus a periodic outbound TCP/UDP
workload (`conn-gen`) so dynamic paths have live flows to discover.
Optionally also installs the **CrowdStrike Falcon** sensor.

Sibling to the Linux [`netpath-vm`](../netpath-vm) scenario — see that
README for the Amazon Linux 2023 variant. This is a deploy utility, not an
E2E test. It is a standalone Pulumi program that uses `pulumi-aws` directly
— no dependency on the e2e-framework, so it works in **any** AWS account
where you have EC2 + IAM permissions.

Access is via **AWS Systems Manager Session Manager** (PowerShell session) —
no RDP, no inbound rules, no SSH.

One Pulumi stack per Datadog environment (e.g. `us1-prod`, `eu1-staging`).

## Config reference

All keys are set with `pulumi config set <key> <value>` (or `--secret` for
secrets). `pulumi config` lists everything currently set on the active stack.

| Key                          | Required? | Default                                     | Notes                                                          |
|------------------------------|-----------|---------------------------------------------|----------------------------------------------------------------|
| `aws:region`                 | Yes\*     | —                                           | \*Or set `AWS_REGION` in the shell. Standard pulumi-aws key.   |
| `netpath:apiKey`             | Yes       | —                                           | Use `--secret`. Datadog API key for the target org.            |
| `netpath:site`               | No        | `datadoghq.com`                             | e.g. `datadoghq.eu`, `us3.datadoghq.com`, `datad0g.com`.       |
| `netpath:name`               | No        | `netpath-vm-windows`                        | EC2 Name tag + base name for IAM/SG resources. Set per stack.  |
| `netpath:instanceType`       | No        | `t3.xlarge`                                 | Any EC2 instance type that supports Windows Server 2022.       |
| `netpath:rootVolumeSizeGiB`  | No        | `60`                                        | Windows + Falcon eat the default 30 GiB; 60+ is comfortable.   |
| `netpath:tags`               | No        | (none)                                      | Comma-separated, e.g. `env:qa,variant:control`.                |
| `netpath:targets`            | No        | embedded `assets/targets.txt`               | Override the conn-gen TCP/UDP destination list.                |
| `netpath:enableScheduled`    | No        | `true`                                      | Boolean. `false` skips the `network_path` integration entirely.|
| `netpath:scheduledConfig`    | No        | embedded `assets/network_path.yaml`         | Override the integration YAML (5 default targets — see assets).|
| `netpath:workers`            | No        | `4`                                         | `network_path.collector.workers` in `datadog.yaml`.            |
| `netpath:agentVersion`       | No        | (latest)                                    | e.g. `7.55.0`. Empty = `datadog-agent-7-latest.amd64.msi`.     |
| `netpath:crowdstrikeMsiUrl`  | No        | —                                           | Use `--secret`. Presigned S3 URL to `FalconSensor_Windows.msi`.|
| `netpath:crowdstrikeCid`     | No        | —                                           | Use `--secret`. Falcon CID. Required iff `crowdstrikeMsiUrl` is set. |

If either `crowdstrikeMsiUrl` or `crowdstrikeCid` is unset, the CrowdStrike
install step is skipped entirely.

## What it provisions

- An IAM role + instance profile with the AWS-managed
  `AmazonSSMManagedInstanceCore` policy (lets the SSM agent register the
  instance and accept Session Manager sessions).
- An egress-only security group (no ingress rules — SSM is outbound-only).
- A Windows Server 2022 EC2 instance (default `t3.xlarge`, 60 GiB gp3 root)
  in your default VPC, on the first available subnet, with a public IP for
  outbound to Datadog and to your conn-gen targets.
- A PowerShell user-data script that, on first boot:
  1. Drops `C:\ProgramData\Datadog\conn-gen-targets.txt` and `conn-gen.ps1`.
  2. If `crowdstrikeMsiUrl` + `crowdstrikeCid` are set, downloads the Falcon
     MSI and installs it silently with the given CID.
  3. Downloads the requested Datadog Agent MSI and installs it silently with
     `APIKEY`, `SITE`, and (optionally) `TAGS`.
  4. Overwrites `C:\ProgramData\Datadog\datadog.yaml` with our
     network-path-aware version (`connections_monitoring.enabled: true`,
     `synthetics.collector.enabled: true`, tunable `workers`).
  5. Overwrites `C:\ProgramData\Datadog\system-probe.yaml`
     (`network_config.enabled: true`, `traceroute.enabled: true`).
  6. Drops `conf.d\network_path.d\conf.yaml` if scheduled tests are enabled.
  7. Restarts the `datadogagent` and `datadog-system-probe` services.
  8. Registers a `conn-gen` scheduled task that runs every 1 minute as SYSTEM.

The full bootstrap transcript lives at `C:\Windows\Temp\bootstrap.log` and
MSI install logs at `C:\Windows\Temp\datadog-agent-install.log` /
`falcon-install.log`.

## Prerequisites

- Pulumi CLI installed.
- `PULUMI_CONFIG_PASSPHRASE` exported in your shell (encrypts secrets in
  stack state).
- AWS Session Manager plugin for the AWS CLI:
  ```bash
  brew install --cask session-manager-plugin
  ```
- An AWS account with:
  - A default VPC (most accounts have one).
  - Permission to create EC2 instances, security groups, IAM roles, and
    instance profiles.
- Working AWS credentials in your shell (`aws-vault`, `AWS_PROFILE`, env
  vars, SSO — whatever `aws sts get-caller-identity` works with).
- A **presigned S3 URL** to the CrowdStrike Falcon Windows MSI and the
  matching **CID**, if you want Falcon installed. Generate the URL with
  something like:
  ```bash
  aws s3 presign s3://your-bucket/FalconSensor_Windows.msi --expires-in 3600
  ```

## Deploying

```bash
cd test/new-e2e/scenarios/netpath-vm-windows

pulumi stack init us1-prod
pulumi config set aws:region us-east-1

# Required
pulumi config set --secret netpath:apiKey <api-key>

# Optional (Datadog)
pulumi config set netpath:site datadoghq.com          # default
pulumi config set netpath:name netpath-win-us1-prod
pulumi config set netpath:agentVersion 7.55.0         # omit for latest
pulumi config set netpath:workers 8
pulumi config set netpath:tags 'env:us1-prod,os:windows,purpose:netpath-demo'

# Optional (CrowdStrike) — both required for Falcon to install
pulumi config set --secret netpath:crowdstrikeMsiUrl 'https://...presigned...'
pulumi config set --secret netpath:crowdstrikeCid    'ABCDEF1234567890-12'

aws-vault exec <profile> -- pulumi up
```

Pulumi prints `vmInstanceId`, `vmPublicIp`, and `ssmCommand` outputs on
success.

## Verifying

The Windows AMI takes ~5–10 minutes to finish first-boot work (sysprep,
EC2Launch, user-data). SSM may respond before the bootstrap script
finishes — retry the session command if your first attempt fails.

```bash
aws-vault exec <profile> -- aws ssm start-session --target <vmInstanceId>
```

Once you're in (drops into `cmd.exe`, type `powershell` to switch shells):

```powershell
# Watch bootstrap progress (until the transcript footer appears)
Get-Content -Wait C:\Windows\Temp\bootstrap.log

# Service health
Get-Service datadogagent, datadog-system-probe, datadog-trace-agent, datadog-process-agent

# conn-gen
Get-ScheduledTask -TaskName conn-gen | Get-ScheduledTaskInfo
Get-WinEvent -LogName 'Microsoft-Windows-TaskScheduler/Operational' -MaxEvents 20 |
    Where-Object { $_.Message -like '*conn-gen*' }

# Agent status
& 'C:\Program Files\Datadog\Datadog Agent\bin\agent.exe' status

# CrowdStrike (if installed)
Get-Service csagent
sc.exe query csagent
```

In the Datadog UI for the configured site:

- **Cloud Network Monitoring → Network** — host appears with flows out to
  the configured TCP/UDP destinations.
- **Network Path Analytics → Path Analyses** — filter by `host:<vm-name>`;
  dynamic + scheduled paths to the targets appear within a few minutes.
- **Synthetic Monitoring → Network Path** — target this host by its tag set
  (e.g. `host:<vm-name>` or your custom tags) when authoring synthetic tests.

If no data shows up, check:

1. `aws-vault exec <profile> -- pulumi stack output` — confirm the VM came up.
2. SSM in and `Get-Content C:\Windows\Temp\bootstrap.log` for errors.
3. Confirm `C:\ProgramData\Datadog\datadog.yaml` has
   `network_path.connections_monitoring.enabled: true`,
   `synthetics.collector.enabled: true`, the right `api_key:` / `site:`,
   and that the agent is restarted.

## Tearing down

```bash
aws-vault exec <profile> -- pulumi destroy
pulumi stack rm us1-prod   # only if you want to drop the stack itself
```

## Customizing

- Override the target list per stack:
  `pulumi config set netpath:targets "$(cat my-targets.txt)"`
- Bump `workers` for higher-throughput dynamic path tests.
- Edit `assets/conn-gen.ps1` for additional probe types (only TCP via
  `Invoke-WebRequest`/`TcpClient` and DNS over UDP are wired up by default).
- Change the scheduled-task cadence at the bottom of `main.go`
  (`New-ScheduledTaskTrigger -RepetitionInterval`).

## Notes

- The instance has a public IP for outbound (Datadog intake, conn-gen
  targets, CrowdStrike cloud) but **no inbound rules at all**. SSM Session
  Manager works outbound-only via the SSM agent baked into the Windows AMI.
- The Datadog Agent MSI is pulled directly from the public
  `ddagent-windows-stable` bucket; the install is unauthenticated. Set
  `netpath:agentVersion` to pin a specific build.
- The CrowdStrike MSI is whatever you point at — the script trusts the
  presigned URL contents. Make sure you're pointing at the right `n-X` ring
  for your tenant.
