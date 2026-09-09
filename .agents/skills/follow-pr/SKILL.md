---
name: follow-pr
description: >-
  Monitor the current PR's GitLab pipeline to completion, then report success or investigate a failure.
  Use when the user asks to follow, babysit, watch, or wait on a PR/pipeline, or just after pushing to / creating a PR.
model: sonnet
---

# Follow PR

Watch the latest Gitlab CI pipeline for the current PR to a terminal state and report the outcome.

## Step 0: Ensure correct environment
The appropriate tool for this usecase is `ddgl`, and more specifically `ddgl attach`.
Check if `ddgl` is available - `which ddgl`. If so, move to [Step 1](#step-1-determine-the-target). Otherwise, use a dev env as specified below.

### Ensuring a dev env
First, check if you are running in a dev env: `test -f /.started` will exit 0 if so. If you are in an outdated devenv without `ddgl`, stop and notify the user to recreate his dev env.
Otherwise, check for the existence of a dev env by using `dda env dev show`.

**If there are existing dev envs**:
- Check if the current repo is properly mounted into that env (`repos` and `extra_(mount|volume)_specs` fields)
- Check the current state of that dev env.

If the environment is already started and contains the right repo, move to [the next step](#using-a-dev-env).
Otherwise, create one by using `./create_devenv.sh`, then use the environment ID printed by the script in subsequent commands.

### Using a dev env
To run commands inside a dev env, use the following template:
```bash
dda env dev run --id <dev-env-id> -- [command]
```
Watch out for space-splitting. For example:
```bash
dda env dev run --id follow-pr-attach-7C2C42F6 -- ddgl attach --detail=normal --follow --plain
```

## Step 1: Determine the target

If the user gave a ref, branch, or pipeline ID, pass it through (`--ref <ref>` or `--pipeline <id>`).
Otherwise omit both — `ddgl attach` resolves the pipeline for the current branch on its own.

## Step 2: Start monitoring

All pipeline discovery, polling, follow/rebind, and timeout handling is covered by the internals of `ddgl attach`.
Do not implement a second polling loop or persist monitoring state of your own.

Check whether you have a long-lived monitoring tool available, one that can run a command in the background and forward each stdout line as it arrives, without a timeout of its own (e.g. Claude Code's `Monitor` tool).

**With such a tool:** start it on

```bash
ddgl attach --plain --follow --detail=full [--ref <ref> | --pipeline <id>]
```

and wait for a `[FINAL]` line — no `--timeout` needed.

**Without one:** run it in the foreground, bounded so the invocation cannot
outlive your own harness timeout:

```bash
ddgl attach --plain --follow --detail=full --timeout 600 [--ref <ref> | --pipeline <id>]
```

If the `[FINAL]` line reports a timeout (not a pipeline outcome), start an identical invocation again.
This is safe: `attach` is stateless and each invocation begins with a fresh snapshot of the pipeline.

> NOTE: If the pipeline is already terminal or does not exist when you start monitoring, the user might have just pushed and the pipeline is still waiting to be created.
> In this case, wait for 60 seconds and then re-attempt monitoring. The `--follow` argument will make sure `ddgl attach` always monitors the latest pipeline for the ref.

## Step 3: Interpret the output

You may see:

- `[POLL]` - rollup summary after a changed poll tick (jobs done/total, stage,
  failure count). Informational only.
- `[INFO]` - an informational log from `ddgl` itself.
- `[PIPE]` - a change in the pipeline status.
- `[JOB]` - a job finished running and changed state.
- `[FINAL]` - the terminal, authoritative outcome. Treat this line as the
  source of truth regardless of the command's exit code — it names the pipeline id, terminal status, and, on failure, the failed job names.

## Step 4: Act on the outcome

- **Pipeline Success:** stop monitoring and report the pipeline succeeded.
- **Some job failed, but the pipeline is still running**:
    Most jobs on `datadog-agent` CI retry once automatically on any failure (`.gitlab-ci.yml`'s
    default `retry: max: 1, when: always`), so a first-attempt failure alone isn't yet evidence
    of anything — GitLab will retry it once. A minority of jobs (most e2e, Windows, macOS —
    `.retry_only_infra_failure`) retry only on GitLab's infra-flavoured `failure_reason` values,
    so a `script_failure` there gets no automatic retry at all and is worth a closer look sooner.
    Unit test, linter, and build failures are less likely to be flakes regardless of policy.
    If you're unsure which policy a job is on: `grep -rn '<job-name>' .gitlab/ .gitlab-ci.yml`.
    Otherwise, ask the user whether to continue monitoring, or if this job failure is already a
    problem. In the latter case, move to [Step 5](#step-5-follow-up-on-failures).
- **Pipeline failed or canceled:** Stop monitoring, report the status, and move to [Step 5](#step-5-follow-up-on-failures).
- **Timeout `[FINAL]`:** re-invoke `ddgl attach` as in Step 2; this is not a true terminal outcome.
- **Unexpected error** (from `ddgl` itself, or from the monitoring tool): report what happened. Do not attempt a recovery action.

## Step 5: follow-up on failures

Invoke `/triage-ci-failure` on the pipeline id from the `[FINAL]` line. It classifies each
failed job as caused by an active incident, infra/platform flakiness, a code regression, or
ordinary flakiness, and ends with a verdict plus an `Incident: ...` line per job.

Act on the verdict in context — there is no file or schema to read back, only the conversation.
The `Incident:` line tells you what to do next:
- **active, still breaking** — continue to [Step 6](#step-6-watch-an-unresolved-incident) andwait it out.
- **stable** or **resolved** — tell the user it's safe to rebase onto `main` and re-run, saying plainly that `stable` is a weaker signal than `resolved` (the fix may still be in progress).
  Investigation ends here.
- **none, or no incident at all** — report the verdict, and if it's PR-caused, the proposed fix (still don't apply it). Investigation ends here.

## Step 6: watch an unresolved incident

Only entered when `/triage-ci-failure` reported an incident that's still **active and breaking** for a failed job — this is the other half of watching a PR through:
the pipeline is red because of something outside the PR, and it will stay red until that something changes.

Poll the incident on an interval (a few minutes is reasonable; don't busy-loop):

```bash
.agents/skills/triage-ci-failure/scripts/incidents.py timeline <IR-nnnnn>
```

Watch for a state transition off `active` — to `stable` (a rollback or workaround has likely landed; rebasing is probably safe even if the root cause isn't fully fixed yet) or `resolved`/`completed` (the stronger signal).
Once either happens, confirm recovery before telling the user to act — check that the job is passing again on `main`:

```bash
pup cicd events aggregate \
  --query='ci_level:job @ci.pipeline.name:DataDog/datadog-agent @git.branch:main @ci.job.name:"<job name>"' \
  --compute=count --group-by='@ci.status' --from='2h'
```

Once `main` is clean, tell the user it's time to rebase onto `main` and re-run.
