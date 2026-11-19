# GPU E2E Tests

## Overview

GPU e2e tests are located in `test/new-e2e/tests/gpu/` and verify GPU monitoring functionality in both host and Kubernetes environments.

## Test Suites

### Host Tests
- `TestGPUHostSuiteUbuntu2204`: Tests GPU monitoring on Ubuntu 22.04 host
- `TestGPUHostSuiteUbuntu1804Driver430`: Tests on Ubuntu 18.04 with driver 430
- `TestGPUHostSuiteUbuntu1804Driver510`: Tests on Ubuntu 18.04 with driver 510

### Kubernetes Tests
- `TestGPUK8sSuiteUbuntu2204`: Tests GPU monitoring in Kubernetes on Ubuntu 22.04
- `TestGPUMokkaK8sSuite`: Tests GPU monitoring in Kubernetes on a CPU-only kind cluster, with two NVIDIA T4 GPUs simulated by Mokka (see [Mokka (simulated GPUs)](#mokka-simulated-gpus))
- `TestGPUMokkaKAISuite`: Tests the KAI Scheduler integration (`kube_scheduler*` and `kai_*` tags on container and allocated-GPU metrics) on the same Mokka setup, with KAI installed by `kindvm.WithKAIScheduler`
- `TestGPUMokkaKueueSuite`: Tests the Kueue integration (`kube_scheduler*` and `kueue_*` tags on container and allocated-GPU metrics) on the same Mokka setup, with Kueue installed by `kindvm.WithKueue`

## Key Components

### Test Files
- `gpu_test.go`: Main test suite definitions and test cases
- `provisioner.go`: Infrastructure provisioning logic
- `capabilities.go`: Environment-specific capabilities (host vs k8s)
- `mokka_provisioner.go`, `mokka_test.go`: Mokka suite (simulated GPUs)
- `mokka_kai_test.go`, `mokka_kueue_test.go`: KAI Scheduler and Kueue suites on Mokka
- `mokka_scheduler_helpers_test.go`: Shared helpers to run a one-GPU Job through a batch scheduler and check its tags

### Test Cases

1. **TestGPUCheckIsEnabled**: Verifies GPU check is enabled and running
2. **TestGPUSysprobeEndpointIsResponding**: Checks sysprobe GPU endpoint
3. **TestLimitMetricsAreReported**: Verifies limit metrics (gpu.core.limit, gpu.memory.limit)
4. **TestVectorAddProgramDetected**: Tests that GPU workloads are detected
5. **TestNvmlMetricsPresent**: Verifies NVML metrics are collected
6. **TestWorkloadmetaHasGPUs**: Checks workloadmeta contains GPU entities
7. **TestCOATMetricsReported**: Checks the COAT `gpu` profile selects the GPU check telemetry, using `diagnose show-metadata agent-telemetry` (core agent only)
8. **TestZZAgentDidNotRestart**: Ensures agent didn't restart during tests
9. **TestMockGPUAllocationTags** (Mokka suite only): A pod requesting one GPU gets its tags on the metrics of exactly one GPU

## Provisioning

### Host Provisioner

The host provisioner (`gpuHostProvisioner`):
1. Creates EC2 GPU instance (g4dn.xlarge)
2. Validates GPU devices are present
3. Configures ECR authentication using the pre-baked credentials helper
4. Configures pre-baked Docker
5. Pre-pulls test images
6. Validates Docker can run CUDA workloads
7. Installs Datadog agent

### Kubernetes Provisioner

The Kubernetes provisioner (`gpuK8sProvisioner`):
1. Creates EC2 GPU instance
2. Validates GPU devices
3. Configures ECR authentication using the pre-baked credentials helper
4. **Configures pre-baked Docker** (required for pre-pulling CUDA image)
5. **Pre-pulls CUDA sanity check image** (avoids ECR auth issues)
6. Creates Kind cluster with NVIDIA GPU operator
7. Deploys Datadog agent via Helm

## Common Issues and Solutions

### ECR Authentication Failures

**Problem**: EC2 instance cannot pull images from ECR during provisioning

**Error**: `User: arn:aws:sts::... is not authorized to perform: ecr:BatchGetImage`

**Root Cause**: Running in the wrong AWS account. E2E tests **must** run in the `agent-sandbox` AWS account.

**Solution**:
1. Verify `~/.test_infra_config.yaml` is configured correctly:
   ```yaml
   configParams:
     aws:
       keyPairName: "your-keypair-name"
       publicKeyPath: "/path/to/your/public/key"
   ```
2. Ensure you're authenticated with the correct account:
   ```bash
   aws-vault login sso-agent-sandbox-account-admin-8h
   ```
3. The default environment is `agent-sandbox` (see `test/new-e2e/pkg/runner/local_profile.go`). If you're in a different account, EC2 instances won't have the correct IAM permissions.

**Note**: The `-c ddagent:imagePullPassword` flags in the test command are for the Kubernetes agent to pull images, not for the EC2 instance. The EC2 instance uses its IAM role, which requires the correct AWS account setup.

## Running Tests

### Kubernetes Tests

```bash
E2E_DEV_MODE=true dda inv -- -e new-e2e-tests.run \
  --targets ./tests/gpu \
  -c ddagent:imagePullRegistry=669783387624.dkr.ecr.us-east-1.amazonaws.com \
  -c ddagent:imagePullPassword=$(aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password) \
  -c ddagent:imagePullUsername=AWS \
  --run TestGPUK8sSuiteUbuntu2204 \
  2>&1 | tee /tmp/gpu_test_output.log
```

### Host Tests

```bash
E2E_DEV_MODE=true dda inv -- -e new-e2e-tests.run \
  --targets ./tests/gpu \
  --run TestGPUHostSuiteUbuntu2204
```

## System Data Configuration

Each system has specific configuration:

```go
gpuSystemUbuntu2204: {
    amiDescriptor:                os.NewDescriptor(os.Ubuntu, "22-04-gpu"),
    os:                           os.Ubuntu2204,
    cudaSanityCheckImage:         "nvidia/cuda:12.6.3-base-ubuntu22.04",
    hasAllNVMLCriticalAPIs:       true,
    supportsSystemProbeComponent: true,
}
```

GPU AMI IDs are maintained in `test/e2e-framework/resources/aws/platforms.json`
under Ubuntu x86_64 (`22-04-gpu`, `18-04-gpu-driver-430`, and
`18-04-gpu-driver-510`). Both provisioners resolve `amiDescriptor` through
`aws.GetAMI`; the separate `os` field preserves runtime compatibility settings.

## Mokka (simulated GPUs)

[Mokka](https://nvidia.github.io/k8s-test-infra/) (NVIDIA `nvml-mock`) makes CPU-only nodes look like
GPU nodes: it stages a mock driver (`libnvidia-ml`, `/dev/nvidiaN`, sysfs) from a GPU profile and
injects it into containers with an NRI plugin. The reusable pieces live in the e2e framework:

- `components/kubernetes/mokka`: installs the `nvml-mock` chart (pinned version) and the NVIDIA
  device plugin, so nodes advertise `nvidia.com/gpu`.
- `kindvm.WithMokka(...)`: installs it before the Agent and adds `mokka.AgentHelmValues()`
  (the `nvml-mock.nvidia.com/devices` pod annotation, and an empty `gpuMonitoring.runtimeClassName`).
  Call it once per GPU pool (`mokka.WithName`, `mokka.WithNodeSelector`) to mix profiles.

What it covers: NVML inventory and metrics, workloadmeta GPUs, GPU tags, GPU allocation to pods
(kubelet pod-resources), and scheduler/queue tagging of GPU pods. What it does not: CUDA
execution, eBPF/system-probe probes, per-process metrics, real values (they are synthetic, assert
presence and tags only). Keep those in the `g4dn` suites.

Gotchas:
- The NRI plugin never injects into `kube-system` or the Mokka namespace, and only into containers
  created after it runs: the Agent must be deployed after Mokka (`kindvm.WithMokka` handles it).
- The Mokka and device plugin images come from `ghcr.io` and `nvcr.io`, so the suite uses
  `kindvm.WithInternetAccess()`.
- The Mokka chart is pulled by Pulumi on the CI runner, which can't reach `ghcr.io`. Until the
  `ghcr.io` ECR pull-through cache exists (ddoghq/cloud-inventory#3478), the Mokka suites run in the
  manual `new-e2e-gpu-mokka` job instead of `new-e2e-gpu`. Locally they work as long as your machine
  can reach `ghcr.io`.
- With NVLink profiles (`gb200`, `gb300`), the `nvlink_fec` collector sends FEC histogram buckets
  with counts of ~10^12 per run, and the aggregator sketch OOM-kills the core Agent within seconds.
  Use `DD_GPU_DISABLED_COLLECTORS=nvlink_fec` with those profiles until that is fixed.

## GPU Instance Type

Tests use `g4dn.xlarge` (the cheapest GPU instance type) with NVIDIA Tesla T4 GPUs.

## Notes

- Tests are **not to be run in parallel** as they wait for checks to be available
- Some tests skip if the system doesn't have all NVML APIs or system-probe support
- Flaky test markers are used for known transient issues (Pulumi errors, unattended-upgrades, rate limits)
