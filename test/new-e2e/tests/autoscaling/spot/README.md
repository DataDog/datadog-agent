# Spot Scheduling E2E Tests

Integration tests for the cluster-agent spot scheduler using a local kind cluster.

## Prerequisites

1. **Docker** and **kind** installed locally
2. A locally-built cluster-agent image

## Build the cluster-agent image

The cluster-agent must be built for Linux (macOS binaries won't run in kind), so run the build
inside the [developer environment](../../../../../doc/tutorials/dev/env.md):

```bash
dda env dev start
dda env dev run -- dda inv deps   # first time only
dda env dev run -- dda inv cluster-agent.hacky-dev-image-build --target-image ${USER}/cluster-agent:test
```

This builds the cluster-agent from your working tree (including uncommitted changes), layers it on top of
the latest released cluster-agent image and produces `${USER}/cluster-agent:test` in the Docker daemon
shared with the host. Use `--base-image` to pin the base image.
See [Test with Local Agent Image](../../../../../doc/how-to/test/e2e/running.md#test-with-local-agent-image)
for details.

## Run the tests

Run from the **repo root**. The test creates the kind cluster automatically and loads the image into it.

```bash
DD_TEST_CLUSTER_AGENT_IMAGE=${USER}/cluster-agent:test \
  PULUMI_CONFIG_PASSPHRASE=dummy \
  dda inv new-e2e-tests.run --targets=./tests/autoscaling/spot/... \
  --run "^TestSpotSchedulingKind$" \
  -e "-test.timeout 25m"
```

The `--run "^TestSpotSchedulingKind$"` filter is required to exclude `TestSpotSchedulingKindCI`,
which runs the same suite on an AWS-provisioned kind VM and is intended for CI pipelines only.
Without the anchored filter, `dda inv` auto-detects the pipeline ID and the CI test runs
locally, competing with the local kind cluster for the same Pulumi stack.

If `DD_TEST_CLUSTER_AGENT_IMAGE` is not set, tests are skipped.

## How it works

1. `localkubernetes.Provisioner` creates a 3-node kind cluster via Pulumi:
   - 1 control-plane node
   - 1 on-demand worker (no capacity-type label)
   - 1 worker labeled `autoscaling.datadoghq.com/capacity-type=interruptible` with a `autoscaling.datadoghq.com/capacity-type=interruptible:NoSchedule` taint
2. The cluster-agent image is loaded into kind via `WithKindLoadImage`.
3. The cluster-agent is deployed via Helm with spot scheduling enabled and short timeouts.
4. Tests create Deployments and StatefulSets, observe pod placement via the k8s API, and verify spot/on-demand ratios.

## Debug

Use `E2E_DEV_MODE=true` to keep the kind cluster alive after test failures so you can inspect pod state directly:

```bash
DD_TEST_CLUSTER_AGENT_IMAGE=${USER}/cluster-agent:test \
  PULUMI_CONFIG_PASSPHRASE=dummy \
  E2E_DEV_MODE=true \
  dda inv new-e2e-tests.run --targets=./tests/autoscaling/spot/... \
  --run "^TestSpotSchedulingKind$"
```

After inspecting, destroy the stack — this deletes the kind cluster and removes all Pulumi state:

```bash
# 1. Find the workspace for the spot scheduling stack
$TMPDIR/pulumi-workspace/*/*spotschedulingsuite*

# 2. Destroy resources (deletes the kind cluster) and remove the stack
PULUMI_CONFIG_PASSPHRASE=dummy pulumi destroy --cwd <workspace> --yes
PULUMI_CONFIG_PASSPHRASE=dummy pulumi stack rm  --cwd <workspace> --yes
```
