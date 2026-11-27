---
name: create-pr
description: Open a pull request (draft by default) for the current branch, with the right labels, a local review by a different AI provider, and a description reviewers can follow. Use it whenever you open a PR in this repo, via /create-pr or `gh pr create`, including as one step of a larger task ("open a PR", "create a pull request", "make a PR", "push this and open a draft"), since CI and reviewers expect the labels, review and description it produces.
allowed-tools: Bash, Read, Glob, Agent, TaskStop, Skill
argument-hint: "[--real] [additional labels...]"
model: sonnet
---

Create a pull request for the current branch following the Datadog Agent contributing guidelines.

**This is the canonical process for opening PRs in this repo.** Whenever you (or another agent or skill working in this repo) are about to run `gh pr create`, not only when the user types `/create-pr`, follow the steps below instead of assembling a title, body and labels ad hoc: an improvised PR misses the labels CI checks and the description reviewers rely on.

**Ownership:** maintained by `@DataDog/agent-devx`.

**Prerequisites:** the `gh` CLI must be installed and authenticated (`gh auth status`) with access to `DataDog/datadog-agent`, and `origin` must point at that repo (or a fork with push access) so `git push` and `gh pr create` succeed. The local review (step 5) needs [`dda`](https://datadoghq.dev/datadog-agent-dev/) and Python 3.9+, plus `codex` or `claude` logged in on the host, or docker for a developer environment. Without them, a subagent reviews instead.

## Instructions

1. **Check the current branch**. Below, `main` stands for the repository default branch.
   - **On `main`**:
     - With uncommitted or staged changes (`git status`): create a feature branch (`git checkout -b <branch-name>`), stage the changes, commit, and push.
     - With no changes at all: stop and tell the user there is nothing to open a PR for.
   - **On a feature branch with an open PR** (`gh pr list --head <branch> --state open`): check whether the uncommitted changes and unpushed commits belong to that PR's work.
     - Related: continue on this branch.
     - Unrelated: give them their own PR rather than piling them onto this one. Create a branch from an up-to-date `main` (`git checkout main && git pull && git checkout -b <branch-name>`), move the changes onto it, commit, and push.
     - Unsure: ask the user.
2. **Get the commits** on this branch compared to `main` using `git log main..HEAD`
3. **Get the diff** using `git diff main...HEAD` (three dots: only this branch's changes, even after `main` moves on) to understand all changes
4. **Read the PR template** from `.github/PULL_REQUEST_TEMPLATE.md`
5. **Local code review**: get a review from another reviewer than yourself, so it isn't anchored on the reasoning that produced the change. The review script runs `dda review`, which builds the same prompt as the PR `Code review` workflow (`.github/workflows/code-review.yml`) from the `codereview_guideline.md` files relevant to the diff. It skips reviewers of your own provider, so pass yours:
   ```bash
   .agents/skills/create-pr/scripts/review.py --self claude   # you are Claude Code: codex reviews
   .agents/skills/create-pr/scripts/review.py --self codex    # you are Codex CLI: claude reviews
   .agents/skills/create-pr/scripts/review.py                 # any other agent: codex, then claude
   ```
   A review takes several minutes, and starting a developer environment for the first time pulls a large image. Run it in the background (e.g. `run_in_background: true` in Claude Code) with a timeout of 20 minutes, so it isn't killed mid-run, and do step 6 meanwhile. The review goes to stdout, and the reasons a reviewer was skipped to stderr.
   - **Exit 0**: show the findings to the user with the draft in step 8, and say which reviewer ran when it wasn't `codex`, since CI uses `codex`. Findings don't block PR creation; fix them first only if the user asks.
   - **Any other exit, or no result after 20 minutes**: follow "When the review doesn't complete" in [references/local-review.md](references/local-review.md). It usually ends in a review by a subagent, so the PR still gets one before it opens.
6. **Describe the functional changes**: invoke the `/describe-functional-changes` skill on this branch, passing any motivation or context the user gave (if your harness can't invoke skills, read and follow `.agents/skills/describe-functional-changes/SKILL.md`). It returns a collapsed "Functional changes" block, usually a Before | After table (or a single "No functional change" line), that says what behaves differently, and for whom. Without it, reviewers have to read every changed line to understand what the PR actually does. It may also return notes, such as unverified claims or likely bugs: show them to the user with the draft in step 8. This step doesn't depend on the review, so do it while the review from step 5 runs.
7. **PR title**: Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) format, prefixed with the general area of change. Examples:
   - `fix(e2e): Fix flaky diagnose test`
   - `feat(logs): Add new log pipeline`
   - `refactor(config): Simplify endpoint resolution`
8. **Draft a concise PR description** from the commits and diff, following "PR Description Guidelines" below: short and plain, like a human wrote it in two minutes.
   - Present the draft body (What does this PR do? / Motivation, at minimum) in your reply, with the review findings from step 5 and the notes from step 6.
   - Fold in corrections or context the user gave during the session (the real motivation, an issue link, a tradeoff worth mentioning).
   - Don't block on confirmation for a draft PR: the user can edit the description in the GitHub UI, which is faster than relaying corrections through you.
   - For a non-draft PR (`--real`), get explicit confirmation first: a PR marked ready for review implies its description is final.
9. **Check for a needed backport** (see "Backport Detection" below) and note the matching `backport/<branch>` label(s) if applicable.
10. **Labels**: Choose appropriate labels (plus any additional labels passed as $ARGUMENTS):
   - If the PR only changes tests, docs, CI config, or developer tooling (no Agent binary code changes), use `changelog/no-changelog` and `qa/no-code-change`
   - If the PR changes Agent binary code: `qa/done` when QA was done before merge, or `qa/rc-required` when it can only be validated on the release candidate. If you don't know which, ask the user.
   - A reno release note is only expected for **customer-facing changes** — not for every PR that touches Agent binary code. See "Release Notes" below for what qualifies; if the change isn't customer-facing, use `changelog/no-changelog`
   - Add any `backport/<branch-name>` labels identified in step 9, or if the user explicitly asks for a specific backport
11. **PR body**: Fill in the PR template sections:
   - **What does this PR do?**: The description from step 8. Must be readable independently, tying back to the changed code. Start with a sentence or two on what the code change is, then add the functional changes block from step 6.
   - **Motivation**: A reason why the change is made. Point to an issue if applicable. Include drawbacks or tradeoffs if any.
   - **Describe how you validated your changes**: How you validated the change (tests added/run, benchmarks, manual testing). Only needed when testing included work not covered by test suites.
   - **Additional Notes**: Any extra context, links to predecessor PRs if part of a chain, notes that make code understanding easier. **Only include this section if there is genuinely useful context to add** — omit it entirely rather than filling it with filler.
12. **Push the branch** to origin if needed
13. **Open the PR**: Now that the title, body, and labels are finalized, open the PR. By default, open as **Draft** using `gh pr create --draft`. If `$ARGUMENTS` contains `--real`, open as a regular (non-draft) PR instead (omit the `--draft` flag). Remove `--real` from `$ARGUMENTS` before processing remaining arguments as labels.
14. Once the PR is pushed, ask the user if they want to follow CI status for this PR. If yes, invoke the `/follow-pr` skill.

## PR Description Guidelines (from CONTRIBUTING.md)

The PR description should incorporate everything reviewers and future maintainers need:
- A description of what is changed
- A reason why the change is made (pointing to an issue is a good reason)
- When testing had to include work not covered by test suites, a description of how you validated your change
- Any relevant benchmarks
- Additional notes that make code understanding easier
- If part of a chain of PRs, point to the predecessors
- If there are drawbacks or tradeoffs, raise them

**Avoid AI slop.** Reviewers can tell when a PR description was auto-generated from a diff — padded, generic, restating the code instead of explaining intent. To avoid this:
- Draft the description yourself (step 8) rather than asking the user to write it.
- Keep it short. A few sentences beat a bulleted essay. Don't restate every changed file — the diff already shows that.
- Say what the change does, not only how. The technical summary says how the code changes; the functional changes block (step 6) says what that changes for Agent users, developers, or CI. Keep both, and don't repeat in one what the other already says.
- Don't pad sections with filler when there's nothing to say (e.g. an empty-but-present "Additional Notes" section, or a "Describe how you validated" filled with "N/A" — omit instead).
- Write plainly, the way the user would describe it in Slack to a teammate, not like a press release ("This PR introduces a robust, comprehensive solution to...").

## Release Notes

Only **customer-facing changes** need a reno release note — something a Datadog customer could notice in the shipped agent. Most PRs, including most that touch Agent binary code, don't need one. When unsure whether a change is customer-facing, mention it to the user rather than defaulting either way. When a note is needed, invoke `/create-release-note` to write it; this section only decides whether one is needed.

**Needs a release note:**
- New customer-facing functionality (e.g. "Add support for collecting X on Y")
- Bug fixes customers can hit (e.g. "Fix panic in the log agent when a tailed file is deleted")
- Visible behavior changes: new/renamed/removed metrics or tags, changed default values
- Deprecations, breaking changes requiring user action, security fixes

**No release note** (use `changelog/no-changelog`):
- Internal refactors, dead-code removal, internal APIs and telemetry
- Test-only, docs-only, CI/CD, invoke tasks, bazel/build, and other developer tooling
- Dependency bumps and code cleanups with no customer-visible impact
- Comments, formatting, and other cosmetic changes

## Backport Detection

Before finalizing labels, determine whether this change likely needs to be backported to a release branch:

1. **List "living" release branch labels**: run `gh label list --search backport/7 --limit 5`. We can limit to 5 as there should not be more than 2 active releases. Equivalently: `gh api repos/{owner}/{repo}/labels --paginate --jq '.[] | select(.name | startswith("backport/7")) | .name + " | " + (.description // "")'`. Only labels whose description mentions automatic backport-PR creation (e.g. "Automatically create a backport PR to ... once the PR is merged") correspond to *active* release branches — the repo keeps many old `<version>.x` branches around that are no longer maintained, so branch existence alone (`git ls-remote --heads origin`) is not a reliable signal.
2. **Compare against the base branch's target milestone**: read `release.json`'s `base_branch` / `current_milestone` to see what's currently in development on `main`. A change merged to `main` is typically only backported if the same fix is needed in a still-supported release (e.g. a bug also present in the currently-shipping minor version).
3. **Decide relevance**: this is a judgment call, not automatic — a new feature usually does not need a backport; a bug fix, security fix, or CI/build resilience fix often does, if the affected release branch(es) still exist and are active. If genuinely unsure, ask the user.
4. **If a backport is warranted**, propose the specific `backport/<version>.x` label(s) to the user for confirmation before adding them — don't silently add backport labels.

## Example

```bash
gh pr create --draft \
  --title "fix(e2e): Fix flaky diagnose test by adding missing fakeintake redirect" \
  --label "changelog/no-changelog" \
  --label "qa/no-code-change" \
  --body "$(cat <<'EOF'
### What does this PR do?

The diagnose E2E test flaked because it hit fakeintake directly instead of
through the redirect the agent config sets up. Route it through the same
path the agent uses.

<details>
<summary>Functional changes</summary>

| Before | After |
| --- | --- |
| The diagnose E2E test reached fakeintake through its own connection, so its result didn't reflect the agent's connectivity | The test goes through the same redirect as the agent |

Unchanged: the agent, and what the test asserts.

</details>

### Motivation

Flaked ~1 in 5 runs on main, blocking merges for unrelated PRs.

### Describe how you validated your changes

Ran the test 20x locally with the fix; no failures (previously failed ~4/20).
EOF
)"
```

Note there is no "Additional Notes" section here — it was omitted because there was nothing beyond the two sections above worth adding, per the "no filler" guidance. The functional changes table has a single row because the PR is small; the line after it tells reviewers they don't need to look for agent behavior changes.

## Usage

- `/create-pr` — creates a draft PR (default)
- `/create-pr --real` — creates a non-draft PR
- `/create-pr --real team/my-team` — non-draft PR with an extra label

## Output

When the PR is open, end with a short report, then ask about following CI (step 14). If the user wants to follow it, defer to `/follow-pr`.

```markdown
Opened draft PR <URL>: `<title>`
- Labels: `changelog/no-changelog`, `qa/no-code-change`
- Local review (codex): 2 findings, not fixed:
  - `pkg/foo/bar.go:42`: <defect> — <failure scenario>
  - …
- Functional changes notes: <unverified claims or likely bugs from step 6, if any>
- Backport: <proposed `backport/<version>.x` label awaiting your confirmation, if any>
```

Say "no findings" when the review found nothing, and name the reviewer: codex, claude, or a subagent. When no review ran, say why. Leave out lines that don't apply. The PR is done when it is open with these labels and description, and the user has seen every review finding and note.
