---
name: ci-visibility
description: Query and analyze GitLab CI pipelines using Datadog CI Visibility MCP tools. Use when users ask about CI status, failed jobs, pipeline issues, deployment jobs, or want to fix CI failures. Also use when searching for specific GitLab job names or understanding pipeline structure.
---

# CI Visibility Skill

This skill enables querying and analyzing GitLab CI pipelines for the datadog-agent repository using the Datadog MCP server's CI Visibility tools.

## Available Tools

### Primary Tools

| Tool | Purpose |
|------|---------|
| `search_datadog_ci_pipeline_events` | Search for pipelines, stages, or jobs |
| `aggregate_datadog_ci_pipeline_events` | Compute statistics (counts, averages, percentiles) |
| `get_datadog_flaky_tests` | Find flaky tests causing CI failures |

### Key Parameters

**ci_level** - Granularity of search:
- `pipeline` - Entire pipeline execution
- `stage` - Pipeline stage (e.g., "build", "test", "deploy")
- `job` - Individual job within a stage
- `step` - Individual step within a job

**Common query filters:**
- `@ci.pipeline.name:"DataDog/datadog-agent"` - Filter to this repository
- `@git.branch:<branch-name>` - Filter by branch
- `@ci.pipeline.id:<id>` - Filter by specific pipeline
- `@ci.status:error` - Only failed items
- `@ci.status:success` - Only successful items
- `@ci.job.name:*keyword*` - Jobs matching a pattern

## Common Workflows

### 1. Find Pipelines for Current Branch

```
Tool: search_datadog_ci_pipeline_events
ci_level: pipeline
query: @ci.pipeline.name:"DataDog/datadog-agent" @git.branch:<branch-name>
from: now-7d
```

### 2. Find All Failed Jobs in a Pipeline

```
Tool: search_datadog_ci_pipeline_events
ci_level: job
query: @ci.pipeline.id:<pipeline-id> @ci.status:error
page_limit: 50
```

### 3. Help User Fix CI Failures

When users ask "help me fix CI" or "why is CI failing":

1. Get current branch: `git branch --show-current`
2. Get current commit: `git rev-parse --short HEAD`
3. Search for failed jobs:
   ```
   ci_level: job
   query: @git.branch:<branch> @ci.status:error
   from: now-24h
   ```
4. For each failed job, examine:
   - `@error.message` - The error summary
   - `@error.domain` - Error category (code, platform, setup)
   - `@error.subdomain` - More specific category (test, build, script)

### 4. Find Deployment Jobs

Search for jobs with "deploy" or "staging" in the name:

```
ci_level: job
query: @ci.pipeline.name:"DataDog/datadog-agent" @ci.job.name:*deploy* @git.branch:<branch>
```

Or for staging specifically:
```
query: @ci.pipeline.name:"DataDog/datadog-agent" @ci.job.name:*staging*
```

### 5. Get Job Details with Error Messages

When searching for failed jobs, the response includes:
- `job_name` - Full job name
- `job_id` - GitLab job ID (for constructing URLs)
- `@error.message` - Error summary
- `@error.domain` / `@error.subdomain` - Error classification
- `duration_seconds` - How long the job ran

### 6. Construct GitLab URLs

From job IDs, construct URLs:
- **Pipeline:** `https://gitlab.ddbuild.io/DataDog/datadog-agent/-/pipelines/<pipeline_id>`
- **Job:** `https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/<job_id>`

## Staging Deployment Guide

**Authoritative documentation:** https://datadoghq.atlassian.net/wiki/spaces/agent/pages/3457679550/How+to+deploy+custom+images+on+staging

### CRITICAL: Two Different Staging Targets

There are TWO different "staging" destinations that are commonly confused:

| Target | Registry | Job | Use Case |
|--------|----------|-----|----------|
| **Public Dev** | `docker.io/datadog/agent-dev` | `dev_branch_multiarch-a7` | Quick testing, external sharing |
| **Internal Staging** | `registry.ddbuild.io/images/datadog-agent` (ECR 727006795293) | `publish_internal_container_image-full` | Internal compute infrastructure, CNAB deployments |

### Public Dev Images (`dev_container_deploy` stage)

**Job:** `dev_branch_multiarch-a7`
**Output:** `docker.io/datadog/agent-dev:<branch-name>-py3`
**Trigger:** Manual, available in all pipelines
**Child pipeline:** Triggers `DataDog/public-images`

This is for quick dev testing but does NOT deploy to internal staging infrastructure.

### Internal Staging Images (`internal_image_deploy` stage)

**Job:** `publish_internal_container_image-full`
**Output:** `registry.ddbuild.io/images/datadog-agent:<branch-name>-full`
**Trigger:** Manual, but **ONLY available in deploy pipelines**
**Child pipeline:** Triggers `DataDog/images`

This is required for deploying to internal compute infrastructure (biscuits cluster, etc.).

### How to Get Internal Staging Images

#### Option 1: Run a deploy pipeline (recommended)
```bash
inv pipeline.run --deploy --here
```
This creates a pipeline with the `internal_image_deploy` stage. Then manually trigger `publish_internal_container_image-full`.

#### Option 2: Manual DataDog/images pipeline
Go to: https://gitlab.ddbuild.io/DataDog/images/-/pipelines/new

Variables to set:
```
IMAGE_NAME = datadog-agent
IMAGE_VERSION = tmpl-v7
RELEASE_TAG = <your-branch-name>
BUILD_TAG = <your-branch-name>
TMPL_SRC_IMAGE = v<pipeline-id>-<commit-sha>-7-full
TMPL_SRC_REPO = ci/datadog-agent/agent
RELEASE_STAGING = true
RELEASE_PROD = false
```

Get `TMPL_SRC_IMAGE` from the `docker_build_agent7_full` job output in your pipeline.

### Checking Which Stages Exist in a Pipeline

Search for stages to verify if `internal_image_deploy` is available:
```
Tool: search_datadog_ci_pipeline_events
ci_level: stage
query: @ci.pipeline.id:<pipeline-id>
```

If you only see `dev_container_deploy` but not `internal_image_deploy`, the pipeline wasn't triggered with `--deploy`.

## GitLab CI Structure

### Key Stages (in order)

1. **container_build** - Build container images
2. **dev_container_deploy** - Deploy to public dev registry (always available)
3. **internal_image_deploy** - Deploy to internal staging ECR (**deploy pipelines only**)
4. **deploy_containers** - Production container deployment
5. **trigger_release** - Release triggers

### Related Repositories

| Repo | Purpose | Triggered By |
|------|---------|--------------|
| `DataDog/datadog-agent` | Main agent repo | Direct |
| `DataDog/public-images` | Public registry publishing | `dev_container_deploy` jobs |
| `DataDog/images` | Internal registry publishing | `internal_image_deploy` jobs |
| `DataDog/k8s-datadog-agent-ops` | CNAB deployment configs | Manual for staging deploys |

## Query Examples

### Find why tests are failing

```
ci_level: job
query: @ci.pipeline.name:"DataDog/datadog-agent" @git.branch:my-branch @ci.job.name:*test* @ci.status:error
from: now-24h
```

### Find longest running jobs

```
Tool: aggregate_datadog_ci_pipeline_events
aggregation: PC95
metric: @duration
ci_level: job
query: @ci.pipeline.name:"DataDog/datadog-agent"
group_by: ["@ci.job.name"]
```

### Check if a specific job exists

```
ci_level: job
query: @ci.pipeline.name:"DataDog/datadog-agent" @ci.job.name:"exact-job-name"
from: now-30d
page_limit: 5
```

## Error Classification

Datadog CI Visibility classifies errors into domains:

| Domain | Subdomain | Meaning |
|--------|-----------|---------|
| `code` | `test` | Test failure |
| `code` | `build` | Build/compilation error |
| `platform` | `setup` | Infrastructure setup issue |
| `platform` | `script` | Script execution error |

## Tips

1. **Always specify ci_level** - Default is `pipeline`, but you usually want `job` for debugging
2. **Use wildcards carefully** - `*deploy*` matches "deploy", "undeploy", "deploy_staging", etc.
3. **Check time range** - Jobs older than 7 days may need `from: now-30d`
4. **Pipeline vs Job IDs** - Pipeline IDs are for the overall run, Job IDs are for individual jobs
5. **Branch name format** - Use exact branch name with slashes: `sopell/my-feature`

## Integration with GitHub

For PRs, also check GitHub Actions status:
```bash
gh pr checks
```

This shows ALL checks (GitLab + GitHub Actions) with required/optional status from branch protection rules.
