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
Otherwise, create one by using `./create_devenv.sh`, the ID will be automatically exported as `ATTACH_DEVENV_ID`

### Using a dev env
To run commands inside a dev env, use the following template:
```bash
dda env dev run --id ${ATTACH_DEVENV_ID} -- [command]
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
> In this case, wait for a minute or two and then re-attempt monitoring. The `--follow` argument will make sure `ddgl attach` always monitors the latest pipeline for the ref.

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
- **Some job failed, but the pipeline is still running**: Ask the user whether to continue monitoring, or if this job failure is already a problem. If it is, move to [Step 5](#step-5-follow-up-on-failures)
- **Pipeline failed or canceled:** Stop monitoring, report the status, and move to [Step 5](#step-5-follow-up-on-failures).
- **Timeout `[FINAL]`:** re-invoke `ddgl attach` as in Step 2; this is not a true terminal outcome.
- **Unexpected error** (from `ddgl` itself, or from the monitoring tool): report what happened. Do not attempt a recovery action.

## Step 5: follow-up on failures
Use other `ddgl` features to investigate failures on the pipeline that failed
Using the pipeline id from the `[FINAL]` line:
1. `ddgl jobs get --pipeline <id> --failed --json` — failed-job metadata.
2. `mkdir -p failures && ddgl logs --pipeline <id> --failed --output failures/` — export failed-job log output to `failures/`
3. Compare the failure evidence against the current PR's diff. Decide
    whether the failure is likely caused by this PR, needs more evidence, or
    is unrelated (e.g. flaky infra, an unrelated pre-existing failure).
4. If PR-caused, propose the smallest concrete fix — do not apply it. If
    not, or inconclusive, report the evidence and your reasoning.
