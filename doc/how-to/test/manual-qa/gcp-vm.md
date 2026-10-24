# GCP VM

Provisions a Google Cloud virtual machine with the Datadog Agent installed.

## Prerequisites

GCP support must be enabled during setup:

```bash
dda inv e2e.setup --with-gcp
```

This adds a `gcp.publicKeyPath` entry to `~/.test_infra_config.yaml`.

## Create

```bash
dda inv gcp.create-vm
```

### Key options

| Flag | Default | Description |
|------|---------|-------------|
| `--stack-name` | `gcp-vm` | Suffix for the Pulumi stack name |
| `--os-family` | `ubuntu` | OS: `ubuntu` (only supported family) |
| `--os-version` | latest | Ubuntu version |
| `--architecture` | `x86_64` | CPU architecture: `x86_64` or `arm64` |
| `--instance-type` | `e2-medium` | GCP machine type |
| `--pipeline-id` | — | Deploy the agent build from a specific GitLab pipeline |
| `--agent-version` | latest | Pin an agent version (e.g. `7.58.0-1`) |
| `--agent-config-path` | — | Path to a local `datadog.yaml` to merge with defaults |
| `--agent-flavor` | — | Agent flavor (e.g. `datadog-fips-agent`) |
| `--install-agent` | `true` | Set to `false` to provision a raw VM without the agent |

### Examples

```bash
# Ubuntu ARM VM on GCP
dda inv gcp.create-vm --architecture=arm64

# GCP VM with a pipeline build
dda inv gcp.create-vm --pipeline-id=12345678
```

## Connect

SSH connection details are printed after the instance is ready.

## Destroy

```bash
dda inv gcp.destroy-vm
```

## Limitations

- Only `ubuntu` is supported as an OS family.
- FakeIntake is not supported for GCP VMs.
