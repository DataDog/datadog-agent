# netpath-vm

Long-lived AWS EC2 **Amazon Linux 2023** VM running the Datadog Agent with
**Cloud Network Monitoring** (NPM) and **Network Path dynamic tests** enabled,
plus a periodic outbound TCP/UDP workload (`conn-gen`) so dynamic paths have
live flows to discover.

> **Windows variant:** for a Windows Server 2022 box (optionally with
> CrowdStrike Falcon, pinnable agent version, tunable `network_path` workers),
> see the sibling [`netpath-vm-windows`](../netpath-vm-windows) project.
> Same SSM-only access pattern, same conn-gen / scheduled-test surface.

This is a deploy utility, not an E2E test. It is a standalone Pulumi program
that uses `pulumi-aws` directly — no dependency on the e2e-framework, so it
works in **any** AWS account where you have EC2 + IAM permissions.

Access to the VM is via **AWS Systems Manager Session Manager** — no SSH
keypair, no SSH ingress rules, no public-IP-CIDR juggling.

One Pulumi stack per Datadog environment (e.g. `us1-prod`, `eu1-staging`).

## Config reference

All keys are set with `pulumi config set <key> <value>` (or `--secret` for
secrets). `pulumi config` lists everything currently set on the active stack.

| Key                         | Required? | Default                                     | Notes                                                          |
|-----------------------------|-----------|---------------------------------------------|----------------------------------------------------------------|
| `aws:region`                | Yes\*     | —                                           | \*Or set `AWS_REGION` in the shell. Standard pulumi-aws key.   |
| `netpath:apiKey`            | Yes       | —                                           | Use `--secret`. Datadog API key for the target org.            |
| `netpath:site`              | No        | `datadoghq.com`                             | e.g. `datadoghq.eu`, `us3.datadoghq.com`, `datad0g.com`.       |
| `netpath:name`              | No        | `netpath-vm`                                | EC2 Name tag + base name for IAM/SG resources. Set per stack.  |
| `netpath:instanceType`      | No        | `t3.2xlarge`                                | Any EC2 instance type that supports eBPF (kernel 4.4+).        |
| `netpath:tags`              | No        | (none)                                      | Comma-separated, e.g. `env:qa,variant:control`.                |
| `netpath:targets`           | No        | embedded `assets/targets.txt`               | Override the conn-gen TCP/UDP destination list.                |
| `netpath:enableScheduled`   | No        | `true`                                      | Boolean. `false` skips the `network_path` integration entirely.|
| `netpath:scheduledConfig`   | No        | embedded `assets/network_path.yaml`         | Override the integration YAML (5 default targets — see assets).|

## What it provisions

- An IAM role + instance profile with the AWS-managed
  `AmazonSSMManagedInstanceCore` policy (lets the SSM agent register the
  instance and accept Session Manager sessions).
- An egress-only security group (no ingress rules — SSM is outbound-only).
- An Amazon Linux 2023 EC2 instance (default `t3.2xlarge`) in your default
  VPC, on the first available subnet, with a public IP for outbound to
  Datadog and to your conn-gen targets.
- A cloud-init user-data script that, on first boot:
  1. Installs `bind-utils` (for `dig`).
  2. Drops `/usr/local/bin/conn-gen.sh`, the targets file, and the systemd
     unit + timer.
  3. Runs Datadog's official agent install script with
     `DD_INSTALL_ONLY=true` so the agent doesn't auto-start with the wrong
     config.
  4. Writes `/etc/datadog-agent/datadog.yaml` (api key, site, tags,
     `network_path.connections_monitoring.enabled: true`).
  5. Writes `/etc/datadog-agent/system-probe.yaml`
     (`network_config.enabled: true`).
  6. Starts `datadog-agent`, `datadog-agent-sysprobe`, and the `conn-gen.timer`
     (firing every 60s).

## Prerequisites

- Pulumi CLI installed.
- `PULUMI_CONFIG_PASSPHRASE` exported in your shell (encrypts the API key
  in stack state).
- AWS Session Manager plugin for the AWS CLI:
  ```bash
  brew install --cask session-manager-plugin
  ```
- An AWS account with:
  - A default VPC (most accounts have one).
  - Permission to create EC2 instances, security groups, IAM roles, and
    instance profiles.
- Working AWS credentials in your shell. With aws-vault:
  ```bash
  aws-vault exec <profile> -- pulumi up
  ```
  …or any other credential source the AWS SDK picks up
  (`AWS_PROFILE`, env vars, SSO session, etc.). The Pulumi-AWS provider does
  not require anything special — whatever `aws sts get-caller-identity` works
  with will work here.

## Deploying

```bash
cd test/new-e2e/scenarios/netpath-vm

pulumi stack init us1-prod

# Region (also picked up from AWS_REGION if set by aws-vault)
pulumi config set aws:region us-east-1

# Required
pulumi config set --secret netpath:apiKey <api-key>

# Optional
pulumi config set netpath:site datadoghq.com         # default
pulumi config set netpath:name netpath-demo-us1-prod
pulumi config set netpath:instanceType t3.2xlarge    # default
pulumi config set netpath:tags 'env:us1-prod,purpose:netpath-demo'
# Override default destinations:
# pulumi config set netpath:targets "$(cat ./assets/targets.txt)"

# Disable the scheduled (integration) network_path tests on this stack:
# pulumi config set netpath:enableScheduled false

aws-vault exec <profile> -- pulumi up
```

For another environment:

```bash
pulumi stack init eu1-prod
pulumi config set aws:region us-east-1
pulumi config set --secret netpath:apiKey <eu-api-key>
pulumi config set netpath:site datadoghq.eu
pulumi config set netpath:name netpath-demo-eu1-prod
pulumi config set netpath:tags 'env:eu1-prod,purpose:netpath-demo'

aws-vault exec <profile> -- pulumi up
```

Pulumi prints `vmInstanceId`, `vmPublicIp`, and `ssmCommand` outputs on
success.

## Verifying

Open an SSM session using the printed `ssmCommand`. Cloud-init can take
2–4 minutes to finish on first boot; if SSM responds before cloud-init is
done, retry the session command after a moment.

```bash
aws-vault exec <profile> -- aws ssm start-session --target <vmInstanceId>
```

Once you're in:

```bash
# Watch cloud-init progress (until "Cloud-init ... finished")
sudo tail -F /var/log/cloud-init-output.log

# Once cloud-init is done:
sudo systemctl is-active datadog-agent
sudo systemctl is-active datadog-agent-sysprobe
sudo systemctl list-timers conn-gen.timer
sudo journalctl -u conn-gen.service -n 30

sudo -u dd-agent datadog-agent status | less
```

In the Datadog UI for the configured site:

- **Cloud Network Monitoring → Network** — host appears with flows out to
  the configured TCP/UDP destinations.
- **Network Path Analytics → Path Analyses** — filter by `host:<vm-name>`;
  dynamic paths to the targets appear within a few minutes.

If no data shows up, check:

1. `aws-vault exec <profile> -- pulumi stack output` — confirm the VM came up.
2. SSM in and grep `/var/log/cloud-init-output.log` for errors.
3. Confirm `/etc/datadog-agent/datadog.yaml` has
   `network_path.connections_monitoring.enabled: true` and the right
   `api_key:` / `site:`.

## Tearing down

```bash
aws-vault exec <profile> -- pulumi destroy
pulumi stack rm us1-prod   # only if you want to drop the stack itself
```

## Customizing

- Override the target list per stack:
  `pulumi config set netpath:targets "$(cat my-targets.txt)"`
- Edit `assets/conn-gen.sh` for additional probe types (only TCP-via-curl
  and DNS-via-dig are wired up by default).
- Change the timer cadence in `assets/conn-gen.timer` (default: 60s).

## Notes

- The instance has a public IP for outbound (Datadog intake + conn-gen
  targets) but **no inbound rules at all**. SSM Session Manager works
  outbound-only via the SSM agent.
- If you need true private-subnet networking with no public IP, you'd need
  VPC endpoints for `com.amazonaws.<region>.ssm`,
  `com.amazonaws.<region>.ssmmessages`, `com.amazonaws.<region>.ec2messages`
  (and a NAT gateway if you still want internet egress). Out of scope here.
