# Signals reference — retry policy, `failure_reason`, and CI Visibility baselines

Load this when working through Step 1 (retry policy, `failure_reason`) or Steps
2–3 (CI Visibility baseline, incident search tier ladder) of `SKILL.md`.

## The default retry policy, and why it changes the flake prior

`.gitlab-ci.yml:172-175` retries every job once on any failure by default:

```yaml
default:
  retry:
    max: 1
    when: always
```

GitLab only surfaces the latest attempt, so **a job showing `failed` in a terminal
pipeline has usually already failed twice.** "It's probably just a flake" is a
weaker hypothesis than it looks — a job doesn't get to stay `failed` by flaking
once.

Two exceptions retry on a narrower trigger, so a `script_failure` there was only
attempted once:
- `.gitlab-ci.yml:1454-1466` `.retry_only_infra_failure` — used by most e2e,
  Windows, and macOS jobs. Retries only on GitLab's infra-flavoured
  `failure_reason` values (below).
- `.gitlab/test/kernel_matrix_testing/common.yml:258-268` — same idea, plus
  `job_execution_timeout`.

When it matters whether a specific job got the default policy or the infra-only
one, grep for it rather than resolving the full CI config — the pipeline
definition is tens of thousands of lines:

```bash
grep -rn '<job-name>' .gitlab/ .gitlab-ci.yml
```

There's no way to see this from `ddgl` either: GitLab's job-list endpoint doesn't
return retried attempts unless asked to, and `ddgl` doesn't ask, so there's no
`retried` field and no history of earlier attempts to inspect there.

## `failure_reason` is a hint, not a verdict

GitLab's infra-flavoured `failure_reason` values:

| Value | Meaning |
|---|---|
| `runner_system_failure` | Runner died or never started |
| `stuck_or_timeout_failure` | GitLab gave up waiting |
| `unknown_failure`, `api_failure`, `scheduler_failure`, `stale_schedule`, `data_integrity_failure` | GitLab-side |

These look infra-flavoured, and often are — but `runner_system_failure` is also
exactly what a misspelled `image:` reference produces, and any of them can equally
be a symptom of an active incident rather than a cause. Treat one of these values
as *suggestive*, and only read it as *conclusive* in conjunction with the PR's
diff: an infra-flavoured `failure_reason` on a PR that touches no CI config and no
test setup is real evidence the failure is unrelated to the diff. On a PR that
*does* touch CI config, it's evidence of nothing yet — go look at what changed.

## CI Visibility baseline: how the job behaves elsewhere

The single most decisive signal available, and cheap to get. For each failed job:

```bash
pup cicd events aggregate \
  --query='ci_level:job @ci.pipeline.name:DataDog/datadog-agent @git.branch:main @ci.job.name:"<exact job name>"' \
  --compute=count --group-by='@ci.status' --from='2d'
```

For parallel/matrix jobs, also run the base-name wildcard form to pull in the whole
shard family (strip from the first `:`, append `*`):

```
@ci.job.name:new-e2e-sbom*  →  error: 31, success: 732, skipped: 30, running: 4, canceled: 1
```

A second pass with `@ci.status:error -@error.domain:provider` excludes
infrastructure-attributed errors, mirroring the convention `tasks/libs/notify/
utils.py:11` already uses for its own CI Visibility deep links.

Run the same query three times, varying only the branch clause:

| Query | Answers |
|---|---|
| `@git.branch:main` | Is it broken on the branch everyone merges into? |
| *(no branch clause)* | Overall flakiness across all branches — a better estimate than `main` alone, which sees comparatively few runs of any one job. |
| `@git.branch:<this branch>` | Did it also fail on earlier pushes of this branch? A failure predating your latest commit points away from that commit. |

**Interpretation** — these are heuristic bands, not thresholds anyone has
calibrated; always report the raw counts alongside whichever band you invoke:

| Signal | Hypothesis |
|---|---|
| high error rate on `main` (roughly half of runs or more) | `upstream` — broken for everyone. Go find the incident. |
| low but persistent error rate across all branches | `flake` — but weigh the retry prior above; a job that burned its one retry and still failed isn't obviously flaky. |
| error rate **rose sharply** in the last day or two | `upstream`, and go straight to incident search. A flakiness spike is itself one of the most common reasons an incident gets declared — it may say "job X is flaky" rather than "job X is broken". |
| reliable everywhere, failing only here | `pr-code` — dependable elsewhere, failed on your branch. |
| no data, or the job never runs on `main` | No baseline. Fall through to reading the log. |

## Incident search tier ladder

`scripts/incidents.py search` takes a job-failure timestamp and window and returns
scored candidate incidents — see the script's own docstring for the mechanics (two
time-bounded queries, unioned, matched against job names). Escalate through these
tiers by changing its flags, not by re-running the same call hoping for a
different answer:

1. **(default)** `--service datadog-agent-ci`, default window (48h back, 6h
   grace).
2. Same, `--window-hours 336` (2 weeks) — covers a branch that's been open long
   enough that its failure was already fixed on `main` well before you looked.
3. `--no-service --text <keyword>` — drops the service filter entirely and
   searches free text instead, using a keyword pulled from the job log (Step 4 of
   `SKILL.md`). This is broader than it sounds but still well-scoped in practice:
   an unfiltered `state:active` query across the whole org returns 800+ incidents,
   but a free-text query like `ECR` or `gitlab runner` returns a few dozen.
