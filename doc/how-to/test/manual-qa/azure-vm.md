# Azure VM

Provisions an Azure virtual machine with the Datadog Agent installed. Use this
to test agent behavior on Azure infrastructure or Windows Server environments.

## Prerequisites

Azure support must be enabled during setup:

```bash
dda inv e2e.setup --with-azure
```

This adds an `azure.publicKeyPath` entry to `~/.test_infra_config.yaml`, which
is required for all Azure VM tasks.

## Create

```bash
dda inv az.create-vm
```

### Key options

| Flag | Default | Description |
|------|---------|-------------|
| `--stack-name` | `az-vm` | Suffix for the Pulumi stack name |
| `--os-family` | `windows` | OS: `windows` or `ubuntu` |
| `--os-version` | latest | OS version for the chosen family |
| `--architecture` | `x86_64` | CPU architecture: `x86_64` or `arm64` |
| `--instance-type` | `Standard_B4ms` | Azure VM size |
| `--pipeline-id` | — | Deploy the agent build from a specific GitLab pipeline |
| `--agent-version` | latest | Pin an agent version (e.g. `7.58.0-1`) |
| `--agent-config-path` | — | Path to a local `datadog.yaml` to merge with defaults |
| `--agent-flavor` | — | Agent flavor (e.g. `datadog-fips-agent`) |
| `--install-agent` | `true` | Set to `false` to provision a raw VM without the agent |

### Examples

```bash
# Ubuntu VM on Azure
dda inv az.create-vm --os-family=ubuntu

# Windows VM with a specific agent version
dda inv az.create-vm --os-family=windows --agent-version=7.58.0-1
```

## Connect

SSH connection details are printed after creation (Linux). For Windows, the
task outputs RDP connection information.

## Destroy

```bash
dda inv az.destroy-vm
```

## Limitations

- Only `windows` and `ubuntu` OS families are supported.
- FakeIntake is not supported for Azure VMs.
