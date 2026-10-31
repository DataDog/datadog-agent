# AWS VM

Provisions an EC2 instance with the Datadog Agent installed. The most common
scenario for testing agent behavior on a host.

## Create

```bash
dda inv aws.create-vm
```

### Key options

| Flag | Default | Description |
|------|---------|-------------|
| `--stack-name` | `aws-vm` | Suffix for the Pulumi stack name |
| `--os-family` | `ubuntu` | OS: `ubuntu`, `debian`, `centos`, `redhat`, `suse`, `windows`, `macos` |
| `--os-version` | latest | OS version for the chosen family |
| `--architecture` | `x86_64` | CPU architecture: `x86_64` or `arm64` |
| `--instance-type` | auto | EC2 instance type (e.g. `t3.medium`) |
| `--ami-id` | — | Use a specific AMI instead of the default for the OS family |
| `--pipeline-id` | — | Deploy the agent build from a specific GitLab pipeline |
| `--agent-version` | latest | Pin an agent version (e.g. `7.58.0-1`) |
| `--agent-config-path` | — | Path to a local `datadog.yaml` to merge with defaults |
| `--local-package` | — | Path to a local `.deb` / `.rpm` / `.msi` package to install |
| `--use-fakeintake` | `false` | Deploy a local mock intake alongside the agent |
| `--install-agent` | `true` | Set to `false` to provision a raw VM without the agent |

### Examples

```bash
# Ubuntu ARM VM with a pipeline build
dda inv aws.create-vm --os-family=ubuntu --architecture=arm64 --pipeline-id=12345678

# Windows VM
dda inv aws.create-vm --os-family=windows

# VM with a local package
dda inv aws.create-vm --local-package=./datadog-agent_7.58.0.deb

# Two parallel environments
dda inv aws.create-vm --stack-name=test-a
dda inv aws.create-vm --stack-name=test-b
```

## Connect

The task prints SSH connection details when the instance is ready.

```bash
# Print connection details again
dda inv aws.show-vm --stack-name=<name>
```

For Windows VMs:

```bash
dda inv aws.get-vm-password --stack-name=<name>
dda inv aws.rdp-vm          --stack-name=<name>
```

## Destroy

```bash
dda inv aws.destroy-vm [--stack-name=<name>]
```

## Limitations

- FakeIntake cannot be deployed without the agent (`--install-agent=true` is required).
- macOS VMs require access to macOS-compatible subnets in the `agent-sandbox` account; the required flags are applied automatically when `--os-family=macos` is used.
