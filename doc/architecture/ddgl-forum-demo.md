# Mini-RFC: ddgl updates and CI recovery skills forum presentation

**Status:** Accepted
**Date:** 2026-10-19
**Presenter and reviewer:** Pierre-Louis Veyrenc

## Overview

Give a ten-minute Agent Community Forum presentation, excluding Q&A: a quick refresher on the previously demonstrated `ddgl`, an explanation of new capabilities and how they work, then a short live demo. The story is **customer feedback → improvements → less manual CI work**, not another general introduction to the CLI.

The live sequence is specifically **`ddgl retry` → automatic retry through the skills → automatic code fix and push through the skills**. Do not tour autonomy modes, configuration, budget accounting, or edge cases.

## Problem Description

The audience has already seen `ddgl`. Repeating pipeline browsing and log-reading demonstrations would displace the new features and the evidence that their feedback influenced the work. Slides should explain the mechanisms; the demo should make a few common workflows concrete.

Full Agent builds and an exhaustive recovery demonstration cannot fit the slot. Use prepared failures in lightweight, explicitly synthetic CI. The previously selected transient fixture remains a stateless 50/50 failure; it does not guarantee recovery on the first retry.

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
| 2. Automatic retry, about 60–90 seconds | Invoke `/follow-pr` on a separate prepared transient-failure scenario; show its diagnosis, targeted retry, and resumed monitoring | The skill decides that a retry is appropriate and performs it |
| 3. Automatic fix, remaining time | Invoke `/follow-pr` on a prepared simple code regression; show the localized fix, validation, push, and replacement pipeline result | The same workflow can repair a clear regression rather than blindly retrying |

Keep the prompt/command for each beat visible, but do not read the entire agent transcript aloud. Highlight only the verdict, action, and result. No separate live log-browser tour, `attach --retry` demonstration, mode comparison, or allowed-failure filter comparison. An allowed-failure job may remain visible as background context; it does not need its own demo beat.

Reserve the final minute for where to get the updates, the command/skill to try, an invitation for further feedback, and timing buffer.

### Prepared scenarios, not a complicated live setup

Use separate prepared targets for the three beats. Do not retry the same jobs manually and through the skill. The skill retry scenario must have no code regression; the fix scenario must have no random blocking job. This preserves the requested order: the current skill prioritizes code fixes when both kinds of failure appear together.

Keep each skill target current on its own ref, with a matching clean checkout where code edits are needed. Do not make the retry scenario an obsolete pipeline on the fix scenario's moving branch: the skill checks for superseding pipelines. Branch/pipeline preparation stays off stage.

The transient job starts from a known failed attempt and has an independent 50/50 result on each subsequent attempt. The code regression has an unchanged, fast check and one obvious source correction, supported by a passing reference revision. Its fix must not weaken the check. Neither scenario builds the Agent, provisions infrastructure, or requires a stateful retry mechanism. Disable GitLab's automatic retries so they do not mask the demonstrated actions.

The demo uses existing tools and runner infrastructure. `ddgl 0.4.0rc1` is installed. The authenticated GitLab version check returned `18.5.7`, which lacks the [native retry-count variable introduced in 19.3](https://docs.gitlab.com/ci/variables/predefined_variables/). This supports keeping the chosen random fixture rather than adding attempt-tracking infrastructure.

### Readiness and fallback

Rehearse the complete sequence: a short job script does not guarantee short runner queues, triage, hooks, or pipeline propagation. The four-minute live segment is a target, not a verified duration. Keep an actual rehearsal recording available and label it explicitly if live operations run too slowly. Do not silently start the agent during the slides and present it as a fresh invocation afterward.

Use the existing default autonomy and recovery limits without explaining their full configuration. A 50/50 retry scenario can exhaust its budget; show the actual outcome rather than adding retries beyond policy. Clearly identify synthetic failures. Do not fabricate production history or force a triage verdict; the unchanged skill's ability to justify a retry from this fixture must be established in rehearsal.

Scope mutations to disposable demo refs and pipelines. Keep credentials out of displayed output, preserve normal commit hooks and git safeguards, and never merge demo CI into production branches. No changes to the skills are proposed here.

## Running the demo CI

The top-level `.gitlab-ci.yml` contains only demo jobs. A ref ending in `-fix` selects `forum_demo:fix`; other refs select `forum_demo:retry`. Both include the intentionally failing, non-blocking `forum_demo:optional`. All jobs have GitLab automatic retries disabled, no dependencies, and a three-minute timeout.

Two local branches provide the scenarios:

- `pierrelouis.veyrenc/ddgl-forum-demo`: the random retry fixture and the working conversion reference.
- `pierrelouis.veyrenc/ddgl-forum-demo-fix`: the same pipeline plus a one-line milliseconds-to-seconds conversion regression. The unchanged check makes the intended fix unambiguous. The branch suffix ensures the agent's subsequent push runs the conversion check again.

Publish both refs when ready:

```bash
git push origin pierrelouis.veyrenc/ddgl-forum-demo pierrelouis.veyrenc/ddgl-forum-demo-fix
```

Prepare two failed retry pipelines on the first ref, retaining their IDs. Use the older one for the CLI command and the latest one for the skill. If the random job passes during preparation, start another pipeline; no attempt is guaranteed to fail. Start the fix pipeline on the `-fix` ref and use a clean checkout of that branch for the autofix skill. Authenticate tools and open the relevant terminal/session tabs before presenting.

```text
ddgl retry --pipeline <older-retry-pipeline-id> --failed
/follow-pr pierrelouis.veyrenc/ddgl-forum-demo
/follow-pr pierrelouis.veyrenc/ddgl-forum-demo-fix
```

Introduce the random fixture as intentional demo behavior, not a code defect to repair. Its source and prior attempt logs provide evidence; there is no production flakiness baseline for these job names. The skill still needs to justify its retry decision.

The local reproducer for the fix scenario is:

```bash
python3 -B .gitlab/forum-demo/check_duration.py
```

It passes on the reference branch and fails on the seeded regression branch. The source to repair is `.gitlab/forum-demo/duration.py`; the check remains unchanged. The demo linter exception list is trimmed to match the reduced pipeline, and the jobs have explicit ownership so normal CI configuration hooks remain enabled.

## Alternatives considered

| Approach | Assessment |
|---|---|
| **Feedback-led slides, then three short demo beats** | Recommended: matches the returning audience and shows the requested new workflows |
| Repeat the general `ddgl` feature tour | Rejected: already presented previously |
| One mixed-failure pipeline, with the demo running throughout the talk | Rejected: takes focus away from the explanation and produces fix-before-retry ordering |

## Open questions

- Which actual feedback examples should anchor slide 2? Capability descriptions are known; customer wording and attribution are not yet supplied.
- What is the actual startup time with the reused Agent Linux build image and shared amd64 runner? Configuration validation alone does not measure runner startup.
- Does the unchanged triage skill justify automatic retries of the synthetic fixture, and can both skill workflows finish within the proposed live segment? These are rehearsal gates; do not promise those timings before observing them.
