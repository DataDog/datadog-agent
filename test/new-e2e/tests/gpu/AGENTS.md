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

## Key Components

### Test Files
- `gpu_test.go`: Main test suite definitions and test cases
- `provisioner.go`: Infrastructure provisioning logic
- `capabilities.go`: Environment-specific capabilities (host vs k8s)

### Test Cases

1. **TestGPUCheckIsEnabled**: Verifies GPU check is enabled and running
2. **TestGPUSysprobeEndpointIsResponding**: Checks sysprobe GPU endpoint
3. **TestLimitMetricsAreReported**: Verifies limit metrics (gpu.core.limit, gpu.memory.limit)
4. **TestVectorAddProgramDetected**: Tests that GPU workloads are detected
5. **TestNvmlMetricsPresent**: Verifies NVML metrics are collected
6. **TestWorkloadmetaHasGPUs**: Checks workloadmeta contains GPU entities
7. **TestZZAgentDidNotRestart**: Ensures agent didn't restart during tests

## Provisioning

### Host Provisioner

The host provisioner (`gpuHostProvisioner`):
1. Creates EC2 GPU instance (g4dn.xlarge)
2. Validates GPU devices are present
3. Installs ECR credentials helper
4. Installs Docker
5. Pre-pulls test images
6. Validates Docker can run CUDA workloads
7. Installs Datadog agent

### Kubernetes Provisioner

The Kubernetes provisioner (`gpuK8sProvisioner`):
1. Creates EC2 GPU instance
2. Validates GPU devices
3. Installs ECR credentials helper
4. **Installs Docker** (required for pre-pulling CUDA image)
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
   aws-vault login sso-agent-sandbox-account-admin
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
    ami:                          "ami-03ee78da2beb5b622",
    os:                           os.Ubuntu2204,
    cudaSanityCheckImage:         "nvidia/cuda:12.6.3-base-ubuntu22.04",
    hasEcrCredentialsHelper:      false, // needs to be installed
    hasAllNVMLCriticalAPIs:       true,
    supportsSystemProbeComponent: true,
}
```

## GPU Instance Type

Tests use `g4dn.xlarge` (the cheapest GPU instance type) with NVIDIA Tesla T4 GPUs.

## Notes

- Tests are **not to be run in parallel** as they wait for checks to be available
- Some tests skip if the system doesn't have all NVML APIs or system-probe support
- Flaky test markers are used for known transient issues (Pulumi errors, unattended-upgrades, rate limits)
