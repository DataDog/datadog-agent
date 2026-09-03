---
name: triage-ci-failure
description: >-
  Classify a failed GitLab CI job as caused by an active incident, infra/platform
  flakiness, a code regression, or ordinary flakiness, backed by evidence, before
  proposing a fix or a retry. Use when a PR's pipeline is red and it isn't obvious
  whether the PR's own changes are at fault — trigger phrases include "triage this
  CI failure", "is this failure my fault", "why did this job fail", "is there an
  incident affecting CI", "should I retry or fix this", "is this job flaky". Also
  invoked by /follow-pr once a pipeline is confirmed failed.
model: sonnet
---

# Triage CI failure

## Goal

Answer one question with evidence, not a hunch: **is this failure caused by this
PR's own changes, or by something outside it** (an incident, a platform blip,
ordinary flakiness)? Two failure modes to avoid: burning an hour investigating a
platform outage as if it were a code bug, and waving away a real regression as
"probably flaky" because CI is noisy. Every verdict below must cite the evidence
that produced it — a bare "looks flaky, retry" or "looks broken, fix it" is not an
acceptable output.

This skill only diagnoses. It proposes a fix, a retry, or a wait — it never applies
a fix, retries a job, or rebases on its own. `.agents/skills/follow-pr/SKILL.md`
invokes this skill once a pipeline is confirmed failed and acts on the verdict from
there, including watching an unresolved incident through to resolution.

## Step 0 — Preflight

Both `ddgl` and `pup` are required, and both live in the same places: locally, or
inside a `dda env dev`. Reuse the fallback already written for `ddgl` in
`.agents/skills/follow-pr/SKILL.md` ("Step 0: Ensure correct environment") rather
than re-deriving it — `pup` lives alongside it, so one fallback covers both tools.

```bash
which ddgl pup
```

This skill requires a `ddgl` build where `--failed` excludes `allow_failure: true`
jobs and `--json` prints `[]` rather than a sentence on an empty result. If Step 1
doesn't behave as documented, upgrade `ddgl` — don't work around it here.

If `pup` is present but not authenticated, either run `pup auth login` (browser
prompts started inside a dev env are forwarded back to the host) or use the
`dd-auth` skill for the non-interactive `DD_API_KEY`/`DD_APP_KEY` path. If `pup`
can't be made to work, say so and continue with Steps 1 and 4 only — Steps 2 and 3
are unavailable, and the verdict should state that limitation rather than silently
producing a weaker one.

## Step 1 — Collect the failures

```bash
ddgl jobs list --failed --json --no-pager [--ref <ref> | --pipeline <id>]
```

An empty `[]` means there's nothing to triage — stop here.

Also fetch pipeline state:

```bash
ddgl pipelines get --json [--ref <ref> | --pipeline <id>]
```

If the pipeline is **still running**, a job you're about to triage may yet be
auto-retried into success — note that in the verdict rather than treating the
failure as final.

For each failed job, look at its `failure_reason`. Treat it as **one input, not a
verdict** — see `references/signals.md` for why (`runner_system_failure` is also
exactly what a misspelled `image:` reference produces). If `failure_reason` is one
of GitLab's infra-flavoured values *and* the PR touches no CI config and no test
setup, that conjunction is decent evidence the failure is unrelated to the diff.
Either way, continue to Step 2 — nothing here should end the investigation early.

## Step 2 — CI Visibility baseline

This is usually the most decisive step, and it comes before incident search on
purpose: a job failing on `main` right now is broken for everyone whether or not
anyone has declared an incident yet, and a job that's flaky everywhere doesn't need
an incident to explain it.

For each failed job, ask how it behaves elsewhere:

```bash
pup cicd events aggregate \
  --query='ci_level:job @ci.pipeline.name:DataDog/datadog-agent @git.branch:main @ci.job.name:"<exact job name>"' \
  --compute=count --group-by='@ci.status' --from='2d'
```

Load `references/signals.md` here for the full recipe — the parallel-job wildcard
form, the infra-exclusion pass, the three-baseline comparison (`main`, all
branches, this branch), and how to read the counts. Come out of this step with a
working hypothesis (`upstream`, `flake`, or `pr-code`) for later steps to confirm
or overturn — not a final verdict.

## Step 3 — Incident correlation

Worth doing thoroughly if Step 2 pointed at `upstream` or `flake`. If Step 2
pointed cleanly at `pr-code`, skip to Step 4 unless something else is nagging at
you.

`scripts/incidents.py`, colocated with this file, owns the fiddly parts: turning a
failure timestamp and window into the right Datadog queries, and matching a job
name against an incident's auto-generated title. Run it rather than reimplementing
the query logic by hand:

```bash
.agents/skills/triage-ci-failure/scripts/incidents.py search \
  --at <job-failure-ISO8601-timestamp> \
  --job '<exact failing job name>' [--job '<another one>' ...]
```

Read the match tier in the output (`exact`, `base`, `prefix`, `token`, `none`) —
anything but `none` is worth reading the timeline for:

```bash
.agents/skills/triage-ci-failure/scripts/incidents.py timeline <IR-nnnnn>
```

This collapses tens of KB of Slack-mirror noise down to the handful of lines that
matter: state transitions and human notes. This is where you find out whether the
incident is already fixed (look for a rollback, a merged fix PR, a transition to
`stable`/`resolved`) or still open. **`state != active` does not mean fixed** —
`stable` just means "under control", and `resolved` (null or not) is the field
that actually says whether anyone has closed it out.

If nothing matches, widen deliberately rather than re-running the same call —
escalate through the tier ladder in `references/signals.md`:
1. *(default, above)* `services:datadog-agent-ci`, default window.
2. Same, a much wider window — for old branches whose failure was fixed on `main`
   long before you rebased onto them.
3. Drop the service filter, search free text instead, using a keyword pulled from
   the job log in Step 4 (an image reference, a host, an endpoint, a bucket name).

## Step 4 — Read the log

Skip this if Step 3 already produced a confident, timeline-corroborated verdict.
Otherwise, work through `references/evidence.md`'s cookbook — it's ordered
cheapest-first specifically so you never dump a 50,000-line trace into context.
You're looking for two things: the command that actually failed and its exit
status, and whether the failure happened in the job's own work or in its setup/
teardown. Prefer error text that names something concrete — that's exactly what
tier 3 of Step 3 needs if you end up going back to it.

## Step 5 — Verdict

State two things separately, because they answer different questions and
collapsing them loses information: **blame** (`pr-code`, `upstream`, `infra`,
`flake`, or `inconclusive`) and, if relevant, **which incident** — with its ID and
state, or explicitly `none`.

| blame | incident | Say this |
|---|---|---|
| `pr-code` | — | Propose the smallest concrete fix. Don't apply it. |
| `upstream` | unresolved | Don't suggest rebasing yet. Report the incident. |
| `upstream` | resolved | Rebase onto latest `main` and re-run. Name the fixing commit/PR if the timeline gave you one. |
| `upstream` | none declared | Say CI is broken on `main` with nothing declared for it — worth surfacing loudly. |
| `infra` | any | Suggest a retry. Note whether the job already burned its one automatic retry (`references/signals.md`). |
| `flake` | any | Suggest a retry, citing the measured cross-branch failure rate from Step 2 as the reason — not just a feeling. |
| `inconclusive` | any | Present the evidence and the two most likely readings. Don't guess past what you found. |

End with a line stating the incident outcome on its own, exactly like one of
these, so a caller like `/follow-pr` can act on it without re-deriving your
reasoning:

```
Incident: IR-59848 (stable, unresolved) — https://app.datadoghq.com/incidents/59848
Incident: none
```

## Reference material

- `references/signals.md` — the retry-policy prior and its exceptions, why
  `failure_reason` is a hint and not a verdict, and the full CI Visibility query
  recipe with the three baselines and how to read them, plus the incident-search
  tier ladder. Load it in Step 1 (retry policy, `failure_reason`) and Steps 2–3
  (CI Visibility, tier ladder).
- `references/evidence.md` — the log-reading cookbook and what to look for. Load
  it in Step 4.
