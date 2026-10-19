# Mini-RFC: ddgl updates and CI recovery skills forum presentation

**Status:** Accepted
**Date:** 2026-10-19
**Presenter and reviewer:** Pierre-Louis Veyrenc

## Overview

Give a ten-minute Agent Community Forum presentation, excluding Q&A: a quick refresher on the previously demonstrated `ddgl`, an explanation of new capabilities and how they work, then a short live demo. The story is **customer feedback → improvements → less manual CI work**, not another general introduction to the CLI.

The live sequence is specifically **`ddgl retry` → automatic retry through the skills → automatic code fix and push through the skills**. Do not tour autonomy modes, configuration, budget accounting, or edge cases.

## Problem Description

The audience has already seen `ddgl`. Repeating pipeline browsing and log-reading demonstrations would displace the new features and the evidence that their feedback influenced the work. Slides should explain the mechanisms; the demo should make a few common workflows concrete.

Full Agent builds and an exhaustive recovery demonstration cannot fit the slot. Use prepared failures in lightweight, explicitly synthetic CI. The transient fixture fails attempts owned by `codesync` and passes attempts owned by another user, making retries deterministic for the demo's push-triggered pipelines.

## Proposed solution

### Slides first: approximately five minutes

| Slide | Content | Purpose |
|---|---|---|
| 1. Quick refresher | `ddgl` brings GitLab CI operations into the terminal and gives agents usable output. One sentence or screenshot, about 30 seconds | Re-establish context, not re-demo existing features |
| 2. What changed following feedback | Pair actual feedback with the relevant additions: CLI retries, retrying while monitoring, allowed-failure handling, and delegated CI recovery | Make the connection between customer input and delivered changes explicit |
| 3. How retries work | `ddgl` selects jobs and requests retries through GitLab; monitoring observes the new attempts. Filtering keeps allowed failures out of `--failed` selections by default | Explain the new CLI behavior without enumerating flags |
| 4. How the skills work | `/follow-pr` watches with `ddgl`; `/triage-ci-failure` examines logs, diffs, CI history and incidents; eligible transient failures are retried, while `/handle-pr-ci-failure` reproduces and fixes clear regressions before pushing | Explain where evidence, decisions, and actions live |

Use a simple flow on the final slide: **watch → diagnose → retry OR reproduce/fix/push → watch again**. The distinction is that CLI retrying follows the user's selection, while skill-driven recovery first diagnoses the failure. Mention bounded actions and verification in one sentence, not a configuration walkthrough. Use the presenter's actual feedback examples; do not invent customer quotes or attribute unconfirmed requests.

### Live demo second: approximately four minutes

| Beat | On screen | Intended takeaway |
|---|---|---|
| 1. Manual retry, about 30 seconds | On a prepared failed pipeline, run `ddgl retry --pipeline <id> --failed` and confirm. Show the new attempt being accepted | Retry from the terminal without returning to the GitLab UI |
| 2. Automatic retry, about 60–90 seconds | Push a fresh commit on the same branch and invoke `/follow-pr`; show the skill's diagnosis, targeted retry, and resumed monitoring after the CodeSync-triggered attempt fails | The skill decides that a retry is appropriate and performs it |
| 3. Automatic fix, remaining time | Once the flaky job passes, the downstream conversion check fails; the same `/follow-pr` invocation investigates, fixes, validates, pushes, and watches the replacement pipeline | One invocation handles a transient failure followed by a code regression |

Keep the prompt/command for each beat visible, but do not read the entire agent transcript aloud. Highlight only the verdict, action, and result. No separate live log-browser tour, `attach --retry` demonstration, mode comparison, or allowed-failure filter comparison. An allowed-failure job may remain visible as background context; it does not need its own demo beat.

Reserve the final minute for where to get the updates, the command/skill to try, an invitation for further feedback, and timing buffer.

### One branch, sequential failures

Use only `pierrelouis.veyrenc/ddgl-forum-demo`. The pipeline has two stages: `retry` runs the identity-controlled transient job and the allowed failure; `fix` runs the conversion check only after the transient job succeeds. The explicit `needs` dependency is an ordering constraint, not an artifact or infrastructure dependency.

This makes the first failure visible to triage the flaky job alone. After its retry succeeds, GitLab can run the conversion check and expose the code regression. The skill's existing preference for fixing code before retrying does not conflict with the desired sequence because the two blocking failures are not exposed together.

The CLI demonstration starts from an already-failed pipeline. After that command completes, push a fresh commit on the same ref and launch `/follow-pr` once. Keep the matching checkout clean and let the skill own subsequent retries and pushes. Starting a new pipeline is distinct from retrying the failed jobs of the old one.

The transient job reads `GITLAB_USER_LOGIN` at execution time: `codesync` fails, and any other identity passes. CodeSync creates the initial pipeline and the pipeline after an autofix push; user-authenticated retries get the retrying user's identity. The expected recovery sequence is therefore retry → fix push → retry, using the default three shared cycles. A pipeline created directly through the GitLab UI or a user-authenticated API call would pass this job immediately; fresh demo pipelines must come from pushes through CodeSync. This identity check is a demo convention, not a general-purpose retry detector.

The code regression has an unchanged, fast check and one obvious source correction, supported by a passing reference commit. Its fix must not weaken the check. The pipeline does not build the Agent, provision infrastructure, or require persistent retry state. Disable GitLab's automatic retries so they do not mask the demonstrated actions.

The demo uses existing tools and runner infrastructure. `ddgl 0.4.0rc1` is installed. GitLab `18.5.7` lacks the [native retry-count variable introduced in 19.3](https://docs.gitlab.com/ci/variables/predefined_variables/), but its [retry implementation assigns the current user](https://github.com/gitlabhq/gitlabhq/blob/v18.5.7/app/services/ci/clone_job_service.rb) to the new attempt, and its [variable builder derives `GITLAB_USER_LOGIN` from the job's user](https://github.com/gitlabhq/gitlabhq/blob/v18.5.7/lib/gitlab/ci/variables/builder.rb). The identity-controlled fixture requires no persistent state or API calls from the job.

### Readiness and fallback

Rehearse the complete sequence: a short job script does not guarantee short runner queues, triage, hooks, or pipeline propagation. The four-minute live segment is a target, not a verified duration. Keep an actual rehearsal recording available and label it explicitly if live operations run too slowly. Do not silently start the agent during the slides and present it as a fresh invocation afterward.

Use the existing default autonomy and recovery limits without explaining their full configuration. Show the actual outcome rather than adding retries beyond policy if an unrelated failure consumes the budget. Clearly identify synthetic failures. Do not fabricate production history or force a triage verdict; the unchanged skill's ability to justify a retry from this fixture must be established in rehearsal.

Scope mutations to the disposable demo branch and its pipelines. Keep credentials out of displayed output, preserve normal commit hooks and git safeguards, and never merge demo CI into production branches. No changes to the skills are proposed here.

## Running the demo CI

The top-level `.gitlab-ci.yml` contains three demo jobs, with GitLab automatic retries disabled and a three-minute job timeout:

```text
retry stage:  forum_demo:retry ────────► fix stage: forum_demo:fix
              forum_demo:optional
              (allowed failure)
```

`pierrelouis.veyrenc/ddgl-forum-demo` contains both the identity-controlled fixture and the seeded conversion regression. The working conversion reference is commit `a7414c73d9e`; commit `1140df9003e` introduces the one-line defect. The unchanged check establishes the intended behavior.

Publish the demo branch when ready:

```bash
git push origin pierrelouis.veyrenc/ddgl-forum-demo
```

Prepare one CodeSync-triggered pipeline where `forum_demo:retry` has failed, retaining its ID. Authenticate `ddgl` as your user and open the clean checkout and relevant terminal/session tabs before presenting. The fixture prints its triggering identity in the job log.

```text
ddgl retry --pipeline <already-failed-pipeline-id> --failed
```

Then start a fresh pipeline through CodeSync with an empty commit and push:

```bash
git commit --allow-empty -m "ci(demo): start recovery demonstration"
git push origin pierrelouis.veyrenc/ddgl-forum-demo
```

Launch the skill once and let it continue from the flaky failure to the regression:

```text
/follow-pr pierrelouis.veyrenc/ddgl-forum-demo
```

Introduce the identity-controlled fixture as intentional demo behavior, not a code defect to repair. Its source and prior attempt logs provide evidence; there is no production flakiness baseline for these job names. The skill still needs to justify its retry decision.

Smoke-check the two fixture outcomes locally:

```bash
GITLAB_USER_LOGIN=codesync python3 -B .gitlab/forum-demo/flaky.py
# Expected exit status: 1.
GITLAB_USER_LOGIN=pierrelouis.veyrenc python3 -B .gitlab/forum-demo/flaky.py
# Expected exit status: 0.
```

The local reproducer for the fix scenario is:

```bash
python3 -B .gitlab/forum-demo/check_duration.py
```

It fails on the seeded demo branch. The source to repair is `.gitlab/forum-demo/duration.py`; the check remains unchanged. The demo linter exception list matches the reduced pipeline, and the jobs have explicit ownership so normal CI configuration hooks remain enabled.

## Alternatives considered

| Approach | Assessment |
|---|---|
| **Feedback-led slides, then three short demo beats** | Recommended: matches the returning audience and shows the requested new workflows |
| Repeat the general `ddgl` feature tour | Rejected: already presented previously |
| Expose the flaky job and code regression in parallel | Rejected: the skill prioritizes the code fix; sequential jobs preserve retry-before-fix ordering |
| Separate retry and fix branches | Rejected: unnecessary switching; one branch and one skill invocation demonstrate the complete sequence |
| Random 50/50 transient failures | Replaced by the job-identity check to make initial failures and user retries predictable |

## Open questions

- Which actual feedback examples should anchor slide 2? Capability descriptions are known; customer wording and attribution are not yet supplied.
- What is the actual startup time with the reused Agent Linux build image and shared amd64 runner? Configuration validation alone does not measure runner startup.
- Does the unchanged triage skill justify automatic retries of the synthetic fixture, and can both skill workflows finish within the proposed live segment? These are rehearsal gates; do not promise those timings before observing them.
