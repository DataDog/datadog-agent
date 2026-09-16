---
name: gpu-live-metric-validation
description: Validate live GPU metrics only on clusters running the Agent version under test.
---

<!-- @format -->

# GPU Live Metric Validation

## Purpose

Run GPU metric validation against Kubernetes clusters that are exclusively
running the Agent version under test during the selected validation window,
then investigate any findings.

## Required input

Ask the user which Datadog orgs to validate before running any queries. The
supported values match `tasks/gpu.py`:

- `prod` (`app.datadoghq.com`)
- `staging` (`ddstaging.datadoghq.com`)

## Validation workflow

1. Ask the user which supported orgs to validate.

2. Run the validator for each selected org:

   ```bash
   dda inv gpu.validate-metrics \
     --org <prod-or-staging>
   ```

   By default, the task derives the Agent image-tag wildcard from the current
   release branch and its latest release-candidate tag. It passes that wildcard
   to the validator, which queries `datadog.agent.running` grouped by
   `kube_cluster_name,image_tag`, selects clusters whose nonempty image tags
   all match the wildcard, and ANDs that cluster selection with the GPU
   configuration filters.

   Use `--agent-version <wildcard>` to override the derived version. Use
   `--metric-filter <filter>` only for an additional scope; it is ANDed with
   the version-derived cluster filter.

3. Record the selected org, Agent version wildcard, lookback window, and
   validation output before interpreting any findings.

## Investigating findings

For follow-up Datadog queries, use `dd-auth` to select the target. `pup`
consumes the injected `DD_API_KEY`, `DD_APP_KEY`, and `DD_SITE`; do not
combine this workflow with `pup --org`.

1. Start from a failing GPU metric and group it by the smallest useful set of
   dimensions, normally `gpu_uuid`, `host`, and `kube_cluster_name`. Add the
   failed tag as a group-by dimension when investigating a tag failure.

   ```bash
   dd-auth --domain <target-domain> -- \
     pup --no-agent metrics query \
     --query='count:gpu.<metric>{<validation-filter>} by {gpu_uuid,host,kube_cluster_name,<failed-tag>}' \
     --from=<validation-window> --to=now
   ```

2. Identify patterns before drawing conclusions: whether failures are limited
   to a GPU architecture, device mode, GPU model, host, cluster, workload, or
   Agent image tag. Confirm the affected cluster's Agent image tag with:

   ```bash
   dd-auth --domain <target-domain> -- \
     pup --no-agent metrics query \
     --query='sum:datadog.agent.running{<cluster-filter>} by {kube_cluster_name,image_tag}' \
     --from=<validation-window> --to=now
   ```

3. Preserve the exact queries and affected GPU/host/cluster identifiers in
   the summary.

## Candidate investigations by failure type

- Missing metric: compare the affected GPU configuration with the metric's
  expected support and query `gpu.device.total` for the same scope to confirm
  that devices were present.
- Missing required tag: group the failing metric by the required tag and
  affected GPU/host/cluster. Compare a related GPU metric to determine whether
  the absence is metric-specific.
- Invalid tag value: group by the invalid tag and affected GPU/host/cluster.
  Check whether a series has multiple values for the same tag key before
  treating a comma-separated value as a single emitted tag value.
- Unknown or extra tag: query a workload/container metric such as
  `container.cpu.usage` for the same pod and container. Compare its tags with
  the GPU metric to determine whether the tag originates from the workload.
- If a workload tag points to a Kubernetes resource, inspect the corresponding
  Kubernetes object and its labels and annotations before assigning a source.

## Constraints

- Use an explicit, bounded validation window.
- Use `dd-auth` to select the target for `pup` queries.
- Do not validate clusters that reported a different nonempty Agent image tag
  in the validation window.
- Do not use `pup --org` with `dd-auth`.
