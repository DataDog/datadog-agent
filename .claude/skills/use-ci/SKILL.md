---
name: use-ci
description: >
  Search for, inspect, and play GitLab CI jobs on the datadog-agent pipeline.
  Use this skill when the user wants to: find a job ID, check job status, play/trigger
  a manual job, wait for a job to finish, or sequence a set of jobs in a pipeline.
  Trigger on mentions of GitLab, CI pipeline, "play the job", "build the image",
  "trigger docker_build", or "wait for CI".
---

# GitLab CI Management

## Auth

Tokens are short-lived. Always refresh before each API call:

```bash
GITLAB_TOKEN=$(dda inv -- auth.gitlab 2>/dev/null | tail -1)
```

## Find the Pipeline ID

From a PR:
```bash
gh pr checks <pr_number> 2>&1 | grep "default-pipeline"
# URL format: https://gitlab.ddbuild.io/datadog/datadog-agent/-/pipelines/<ID>
```

## Find Job IDs

Pipelines have hundreds of jobs — paginate (up to 5 pages):

```bash
for page in 1 2 3 4 5; do
  result=$(curl -s --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
    "https://gitlab.ddbuild.io/api/v4/projects/datadog%2Fdatadog-agent/pipelines/<PIPELINE_ID>/jobs?per_page=100&page=$page")
  echo "$result" | jq -r '.[] | [.id, .status, .name] | @tsv' 2>/dev/null || break
done
```

Filter by keyword:
```bash
... | grep -i "docker_build\|publish_internal\|dev_branch"
```

## Check Job Status

```bash
curl -s --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "https://gitlab.ddbuild.io/api/v4/projects/datadog%2Fdatadog-agent/jobs/<JOB_ID>" \
  | jq -r '[.status, .name] | @tsv'
```

## Play a Job

```bash
curl -s -X POST --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "https://gitlab.ddbuild.io/api/v4/projects/datadog%2Fdatadog-agent/jobs/<JOB_ID>/play" \
  | jq -r '.status // .message'
```

## Job States

| State | Meaning | Action |
|-------|---------|--------|
| `created` | Waiting on upstream dependencies | Poll — do NOT try to play yet (returns `400 Unplayable Job`) |
| `manual` | Needs explicit trigger | Play immediately |
| `pending` | Queued for a runner | Wait |
| `running` | In progress | Wait |
| `success` | Done | Proceed to next job |
| `failed` / `canceled` | Terminal error | Notify user and stop |

**Key rule:** A `400 Unplayable Job` response on a `created` job means its dependencies haven't finished yet — just poll every 2 minutes, it will become playable automatically.

## Polling Pattern

When a job needs to be waited on, poll every 2 minutes rather than in a tight loop.
Use `ScheduleWakeup` with `delaySeconds=120` to avoid burning context on repeated checks.
