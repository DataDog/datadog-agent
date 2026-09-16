# Gensim EKS benchmark scenario

`aws/gensim-eks` is a manual workflow for generating Gensim episode data used
by Agent benchmark evaluations. It provisions a persistent EKS cluster and
runs an orchestrator Job that deploys the selected episodes and Agent image.

This scenario is intentionally not run by the E2E CI suite. Do not remove it
because it has no CI callers: benchmark maintainers submit runs manually when
they need new recordings or want to evaluate an Agent image.

## Prerequisites

- Complete the [E2E framework setup](../../README.md#quick-start-guide),
  including access to the Agent sandbox AWS account.
- Have a local checkout of the `gensim-episodes` repository. Set
  `GENSIM_REPO_PATH` to its root when it is not a sibling of this repository.
- Build or otherwise make available the Agent image to evaluate.

Each episode must have a Helm chart and the requested scenario YAML. Episodes
are selected as `episode:scenario` pairs; the task searches the `postmortems`
and `synthetics` directories in the Gensim checkout.

## Submit and monitor a run

Submit one or more episodes with an Agent image:

```bash
dda inv aws.eks.gensim.submit \
  --image=docker.io/datadog/agent-dev:my-tag \
  --episodes=authcore-pgbouncer:pool-saturation
```

Alternatively, pass `--episode-manifest=path/to/manifest.json`. The manifest is
a JSON array of `episode` and `scenario` entries and may pin an episode `sha`
to make results comparable across runs.

The default mode, `record-parquet`, records observer data for offline
testbench replay. Use `--mode=live-anomaly-detection` to send live anomaly
events to Datadog, or `--mode=live-and-record` to do both. Pass `--s3-bucket`
when parquet results should be uploaded to S3.

For episodes containing `docker-compose.yaml`, the submit task builds and
pushes their images to ECR using its Amazon Linux ECS builder. Use
`--skip-build` only when the required images are already cached in ECR.

Monitor progress and inspect the individual episode states with:

```bash
dda inv aws.eks.gensim.status
```

The submit task also prints a `kubectl logs -f` command for the orchestrator.
Use `--stack-name` and `--namespace` consistently on submit, status, and
cleanup commands when overriding their defaults.

## Cleanup

Keep the EKS cluster between runs to avoid reprovisioning it. To stop a run and
remove its workloads while retaining the cluster:

```bash
dda inv aws.eks.gensim.stop-all
```

Destroy the persistent cluster when it is no longer needed:

```bash
dda inv aws.eks.gensim.destroy
```

Both commands accept `--stack-name`; `stop-all` also accepts `--namespace`.
