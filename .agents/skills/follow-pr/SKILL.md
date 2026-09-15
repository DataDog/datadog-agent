---
name: follow-pr
description: >-
  Monitor the current PR's GitLab pipeline to completion, then report success, auto-fix, or investigate a failure.
  Use when the user asks to follow, babysit, watch, or wait on a PR/pipeline, or just after pushing to / creating a PR.
argument-hint: "[<ref> | --pipeline <id>] [--fix-mode autofix|no-autofix|ask] [--max-fix-cycles N] [--policy TEXT]"
model: sonnet
---

# Follow PR

Watch the latest Gitlab CI pipeline for the current PR to a terminal state, report the outcome, and — for failures reliably caused by this PR — autonomously fix and push bounded ones under the resolved autonomy policy while investigating the rest locally.

**Owning team:** `@DataDog/agent-devx`

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

## Step 2: Resolve the autonomy policy

Before starting the monitoring loop, resolve how this run should handle failures caused by the PR's own code:

```bash
python3 .agents/skills/follow-pr/scripts/config.py resolve \
  [--mode <from --fix-mode arg>] [--max-fix-cycles <from arg>] [--policy <from arg>]
```

Pass through any `--fix-mode`, `--max-fix-cycles`, or policy text given in this invocation; otherwise the script falls back to environment variables, then worktree-local config, then global config, then its own default (`autofix`).

- **Resolved mode is `autofix` or `no-autofix`:** report the resolved mode, cycle budget, and whether a custom policy is active, then continue to [Step 3](#step-3-start-monitoring).
- **Resolved mode is `ask`, or the script errors:** ask the user directly, before monitoring starts, whether PR-caused failures this run should be fixed and pushed (`autofix`) or only investigated locally (`no-autofix`). Offer to persist the answer (worktree-local or global config) if they don't want to be asked again; otherwise use it for this run only.

Keep the resolved mode, cycle budget (default `2`), and policy text in context — you'll pass them straight through as `--mode`/`--max-fix-cycles`/`--policy` to subskills that might need it.

## Step 3: Start monitoring

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

## Step 4: Interpret the output

You may see:

- `[POLL]` - rollup summary after a changed poll tick (jobs done/total, stage,
  failure count). Informational only.
- `[INFO]` - an informational log from `ddgl` itself.
- `[PIPE]` - a change in the pipeline status.
- `[JOB]` - a job finished running and changed state.
- `[FINAL]` - the terminal, authoritative outcome. Treat this line as the
  source of truth regardless of the command's exit code — it names the pipeline id, terminal status, and, on failure, the failed job names.

## Step 5: Act on the outcome

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
    problem. In the latter case, move to [Step 6](#step-6-follow-up-on-failures).
- **Pipeline failed or canceled:** Stop monitoring, report the status, and move to [Step 6](#step-6-follow-up-on-failures).
- **Timeout `[FINAL]`:** re-invoke `ddgl attach` as in Step 3; this is not a true terminal outcome.
- **Unexpected error** (from `ddgl` itself, or from the monitoring tool): report what happened. Do not attempt a recovery action.

## Step 6: follow-up on failures

Invoke `/triage-ci-failure` on the pipeline id from the `[FINAL]` line; it returns one `CI triage result` block per failed job.

Route each block by its `Blame` field — never by whether `Incident` happens to be `none`, since that value alone doesn't tell you the job wasn't PR-caused:

- **`upstream`:** if `Incident` is active and still breaking, continue to [Step 7](#step-7-watch-an-unresolved-incident) and wait it out. If `stable` or `resolved`, tell the user it's safe to rebase onto `main` and re-run — say plainly that `stable` is a weaker signal than `resolved` (the fix may still be in progress). If no incident is declared at all, say CI looks broken on `main` with nothing declared for it — worth surfacing loudly. Investigation ends here for that job.
- **`infra` or `flake`:** report the verdict and its suggested action (typically a retry, citing the evidence `/triage-ci-failure` gave you). Investigation ends here for that job.
- **`inconclusive`:** report the evidence and the two most likely readings. Investigation ends here for that job.
- **`pr-code`:** collect every `pr-code` block from this pipeline and invoke `/handle-pr-ci-failure` once with all of them together, passing `--mode`/`--max-fix-cycles`/`--policy` set to the values resolved in [Step 2](#step-2-resolve-the-autonomy-policy). Continue to [Step 8](#step-8-decide-whether-to-keep-going) with its result.

## Step 7: watch an unresolved incident

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

## Step 8: decide whether to keep going

Read `/handle-pr-ci-failure`'s result block:

- **`Outcome: pushed`:** it consumed one fix cycle. Work through these checks in order:
  1. Compare this pipeline's `Failure signatures` against any push from earlier in this same run. If they match, the earlier fix didn't work — don't push again; report that and hand the fresh triage result back to `/handle-pr-ci-failure` so it investigates rather than fixes.
  2. If the cycle budget is already spent, report that and stop — don't push a further fix even if it looks safe.
  3. Otherwise, go back to [Step 3](#step-3-start-monitoring) to watch the replacement pipeline at the `Pushed SHA`, then return here through [Step 6](#step-6-follow-up-on-failures) once it finishes.
- **`Outcome: committed-not-pushed`:** report the local commit and the remaining complex root cause(s) blocking a push, then stop and let the user decide.
- **`Outcome: needs-user` or `blocked`:** report the evidence and the specific question `/handle-pr-ci-failure` asked for, then stop.

Every trip back through this loop re-runs `/triage-ci-failure` from scratch on the new pipeline — never reuse an earlier verdict for a different pipeline.

## Examples

- A missed rename breaks a lint job. `/triage-ci-failure` returns one `pr-code` block; `/handle-pr-ci-failure` classifies it `safe`, fixes it, verifies with `dda inv linter.go`, commits, and pushes. Step 8 sees `Outcome: pushed` (cycle 1 of 2), goes back to Step 3, and the replacement pipeline goes green.
- A test fails intermittently under `-race`. `/handle-pr-ci-failure` classifies it `complex`, reproduces it locally, tries two distinct hypotheses, and stops with `Outcome: needs-user` and an uncommitted candidate diff — nothing is pushed.
- A pushed fix's replacement pipeline fails again with the same `Failure signature`. Step 8 recognizes the repeat, does not push a second attempt, and routes back into `/handle-pr-ci-failure` to investigate instead.
