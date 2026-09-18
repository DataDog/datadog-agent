---
name: triage-ci-failure
description: >-
  Classify a failed CI as either caused by an active incident, flakiness, or a true code regression. 
  Use when a PR's pipeline is red and it isn't obvious whether the PR's own changes are at fault.
  Trigger phrases include:
  - "investigate this CI failure"
  - "please fix CI"
  - "why did this job fail"
  - "is there an incident affecting CI"
  - "should I retry this"
  This should also be invoked whenever the user asks you to investigate _or fix_ a failing CI, to ensure we don't spend hours trying to fix something broken upstream.
model: sonnet
---

# Triage CI failure

## Goal

Answer one question: **is this failure caused by this PR's own changes ?** with hard evidence.
Every verdict below must cite the evidence that produced it — a bare "looks flaky, retry" or "looks broken, fix it" is not an acceptable output.

This skill only diagnoses. Never take action (writing a fix, retrying a job) on your own: only present your investigation results to the user.

## Step 0 — Preflight

Both `ddgl` and `pup` are required, and both live in the same places: locally, or inside a `dda env dev`.

```bash
which ddgl pup
```

If `pup` is present but not authenticated, either run `pup auth login` or use the `dd-auth` skill.
If `pup` can't be made to work, say so and continue with Steps 1 and 4 only:
Steps 2 and 3 are unavailable, and the verdict should state that limitation rather than silently producing a weaker one.

## Step 1 — Collect the failures

```bash
ddgl jobs list --failed --json --no-pager [--ref <ref> | --pipeline <id>]
```

An empty `[]` means there's nothing to triage — stop here.

Also fetch pipeline state:

```bash
ddgl pipelines get --json [--ref <ref> | --pipeline <id>]
```

If the pipeline is **still running**, a job you're about to triage may yet be auto-retried into success.
Note that in the verdict rather than treating the failure as final.

For each failed job, look at its `failure_reason`, i.e. the failure reason as determined by gitlab.
Treat it as an aditionnal data point, not the be-all-end-all. For example, a `runner_system_failure` can be caused by a change of this PR (e.g. a malformed `image:`).
See @references/signals.md for more details.

## Step 2 — CI Visibility baseline

Check if this job is failing _everywhere_ (i.e. on `main`), or if it is often flaky using CI Visibility.

For each failed job, ask how it behaves elsewhere:

```bash
pup cicd events aggregate \
  --query='ci_level:job @ci.pipeline.name:DataDog/datadog-agent @git.branch:main @ci.job.name:"<exact job name>"' \
  --compute=count --group-by='@ci.status' --from='2d'
```

Check @references/signals.md for additional queries that can help if this first one is inconclusive.
Come out of this step with a working hypothesis (`upstream`, `flake`, or `pr-code`) for later steps to confirm or overturn — not a final verdict.

## Step 3 — Incident correlation

If Step 2 pointed clearly at `pr-code`, skip to Step 4.

Use the helper script to search for an active CI incident matching the failing job:

```bash
.agents/skills/triage-ci-failure/scripts/incidents.py search \
  --at <job-failure-ISO8601-timestamp> \
  --job '<exact failing job name>' [--job '<another one>' ...]
```

Read the match tier in the output (`exact`, `base`, `prefix`, `token`, `none`) — anything but `none` is worth reading the timeline for:

```bash
.agents/skills/triage-ci-failure/scripts/incidents.py timeline <IR-nnnnn>
```

This is where you find out how far along the fix is — not just whether one exists.
- `stable` usually means a rollback or workaround has already landed and the affected job(s) should pass again on a rebase.
- `resolved` (or `completed`) is the stronger signal: the incident is fully closed out.

Look for a rollback, a merged fix PR, or an explicit state transition to tell which.

If nothing matches, widen deliberately rather than re-running the same call — escalate through the tier ladder in `references/signals.md`:
1. *(default, above)* `services:datadog-agent-ci`, default window.
2. Same, a much wider window — for old branches whose failure was fixed on `main` long before you rebased onto them.
3. Drop the service filter, search free text instead, using a keyword pulled from the job log in Step 4 (an image reference, a host, an endpoint, a bucket name).

## Step 4 — Read the log

Always do a quick sanity check here, even when Step 3 was conclusive — a time-and-name correlation is strong evidence but not proof.
Skim the job's diff against `main` and the last ~50 lines of its log, and confirm the failure signature actually looks like what the incident describes.

You can obtain the job's log via `ddgl`:
```bash
ddgl logs --job <ID> [--output <some_file>]
```

If it lines up, you're done — the full cookbook below is skippable.
If it doesn't, or Step 3 didn't produce a confident match at all, work through @references/evidence.md's cookbook.

You're looking for two things:
1. the command that actually failed and its exit status
2. whether the failure happened in the job's own work or in its setup/teardown.

## Step 5 — Verdict

State your verdict among the below options, as well as a recommended course of action and the linked incident if any.

| blame | incident status | Suggested action |
|---|---|---|
| `pr-code` | — | Propose the smallest concrete fix. Don't apply it. |
| `upstream` | active, still breaking | Don't suggest rebasing yet. Report the incident. |
| `upstream` | stable | Suggest a rebase and retry — `stable` usually means a rollback or workaround already landed — but say plainly that this is a weaker signal than `resolved`: the underlying fix may still be in progress. |
| `upstream` | resolved | Rebase onto latest `main` and re-run with confidence. Name the fixing commit/PR if the timeline gave you one. |
| `upstream` | none declared | Say CI looks broken on `main` with nothing declared for it — worth surfacing loudly. |
| `infra` | any | Suggest a retry. Note whether the job already burned its one automatic retry (`references/signals.md`). |
| `flake` | any | Suggest a retry, citing the measured cross-branch failure rate from Step 2 as the reason — not just a feeling. |
| `inconclusive` | any | Present the evidence and the two most likely readings. Don't guess past what you found. |

End with a line stating the incident outcome on its own, exactly like one of
these, so a caller like `/follow-pr` can act on it without re-deriving your
reasoning:

```
Incident: IR-59848 (active, still breaking) — https://app.datadoghq.com/incidents/59848
Incident: IR-59848 (stable, probably safe to retry) — https://app.datadoghq.com/incidents/59848
Incident: IR-59848 (resolved) — https://app.datadoghq.com/incidents/59848
Incident: none
```
