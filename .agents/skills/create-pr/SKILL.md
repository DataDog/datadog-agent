---
name: create-pr
description: Create a pull request for the current branch with proper labels and description. Any agent opening a PR in this repo (via `gh pr create` or otherwise), whether invoked directly as /create-pr or as part of a larger task, MUST follow this skill's process rather than improvising.
disable-model-invocation: true
allowed-tools: Bash, Read, Glob
argument-hint: "[--real] [additional labels...]"
model: sonnet
---

Create a pull request for the current branch following the Datadog Agent contributing guidelines.

**This is the canonical process for opening PRs in this repo.** `disable-model-invocation: true` only means the harness won't auto-invoke this skill as a tool call — it does not mean the process below is optional. Whenever you (or another agent/skill working in this repo) are about to run `gh pr create` for any reason — not just when the user explicitly types `/create-pr` — read and follow the steps below instead of assembling a title/body/labels ad hoc.

**Ownership:** maintained by `@DataDog/agent-devx`.

**Prerequisites:** the `gh` CLI must be installed and authenticated (`gh auth status`) with access to `DataDog/datadog-agent`, and `origin` must point at that repo (or a fork with push access) so `git push` and `gh pr create` succeed.

## Instructions

1. **Check the current branch**.
   - If the current branch is `main` (or the default branch):
     - Check for uncommitted or staged changes (`git status`). If there are changes, create a new feature branch from the default branch (`git checkout -b <branch-name>`), stage the changes, commit, and push.
     - If there are no changes at all, stop and inform the user there is nothing to open a PR for.
   - If the current branch is a feature branch that already has an open PR (`gh pr list --head <branch> --state open`), check whether the uncommitted/staged changes and unpushed commits are actually related to that PR's existing work. If they are a distinct, unrelated change, don't pile them onto the existing PR/branch — create a new feature branch from the default branch (`git checkout main && git pull && git checkout -b <branch-name>`), then cherry-pick/move the relevant changes onto it, commit, and push, so the unrelated change gets its own PR. If genuinely unsure whether the changes are related, ask the user.
2. **Get the commits** on this branch compared to `main` using `git log main..HEAD`
3. **Get the diff** using `git diff main..HEAD` to understand all changes
4. **Read the PR template** from `.github/PULL_REQUEST_TEMPLATE.md`
5. **Codex review** (optional): Check if `codex` is installed (`command -v codex`). If it is, run a review against the default branch:
   ```bash
   DEFAULT_BRANCH=$(git rev-parse --abbrev-ref origin/HEAD | sed 's|^origin/||')
   codex review --base "$DEFAULT_BRANCH"
   ```
   `codex review` is an LLM-based review and can take several minutes — do not bound it with a short Bash `timeout`, and do not let it get killed mid-run. Run it with `run_in_background: true` and wait for completion (or pass a generous `timeout`, e.g. the Bash tool max of 600000ms). If it's still running when you check back, keep waiting rather than treating it as done or skipping it, up to that 10-minute ceiling — if it still hasn't finished by then, note that to the user and proceed without it rather than stalling PR creation indefinitely. Show the review output to the user. If codex is not installed, skip this step silently.
6. **PR title**: Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) format, prefixed with the general area of change. Examples:
   - `fix(e2e): Fix flaky diagnose test`
   - `feat(logs): Add new log pipeline`
   - `refactor(config): Simplify endpoint resolution`
7. **Draft a concise PR description** from the commits and diff, then show it to the user for confirmation before opening the PR. See "PR Description Guidelines" below — keep the draft short and plain, like a human wrote it in two minutes, not AI-generated prose. Present the draft body (What does this PR do? / Motivation, at minimum) directly in your reply and ask the user to either confirm it as-is or give corrections/missing context (e.g. the real motivation, an issue link, a tradeoff worth mentioning) — don't ask an open-ended "what does this PR do?" question that puts the writing burden back on them. Fold any corrections in before proceeding. **Do not open the PR until this confirmation is done.**
8. **Check for a needed backport** (see "Backport Detection" below) and note the matching `backport/<branch>` label(s) if applicable.
9. **Labels**: Choose appropriate labels (plus any additional labels passed as $ARGUMENTS):
   - If the PR only changes tests, docs, CI config, or developer tooling (no Agent binary code changes), use `changelog/no-changelog` and `qa/no-code-change`
   - If the PR changes Agent binary code and QA was done, use `qa/done`
   - If the PR changes Agent binary code, a reno release note is expected (remind the user)
   - Add any `backport/<branch-name>` labels identified in step 8, or if the user explicitly asks for a specific backport
10. **PR body**: Fill in the PR template sections:
   - **What does this PR do?**: A clear description of what is changed, based on the user's concise description from step 7. Must be readable independently, tying back to the changed code.
   - **Motivation**: A reason why the change is made. Point to an issue if applicable. Include drawbacks or tradeoffs if any.
   - **Describe how you validated your changes**: How you validated the change (tests added/run, benchmarks, manual testing). Only needed when testing included work not covered by test suites.
   - **Additional Notes**: Any extra context, links to predecessor PRs if part of a chain, notes that make code understanding easier. **Only include this section if there is genuinely useful context to add** — omit it entirely rather than filling it with filler.
11. **Push the branch** to origin if needed
12. **Open the PR**: Now that the title, body, and labels are finalized and confirmed, open the PR. By default, open as **Draft** using `gh pr create --draft`. If `$ARGUMENTS` contains `--real`, open as a regular (non-draft) PR instead (omit the `--draft` flag). Remove `--real` from `$ARGUMENTS` before processing remaining arguments as labels.
13. Once the PR is pushed, ask the user if they want to follow CI status for this PR. If yes, invoke the `/follow-pr` skill.

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
- Draft the description yourself (step 7), but always show it to the user and let them correct or add context before it's final — don't ship your first draft unchecked, and don't outsource the writing to the user either.
- Keep it short. A few sentences beat a bulleted essay. Don't restate every changed file — the diff already shows that.
- Don't pad sections with filler when there's nothing to say (e.g. an empty-but-present "Additional Notes" section, or a "Describe how you validated" filled with "N/A" — omit instead).
- Write plainly, the way the user would describe it in Slack to a teammate, not like a press release ("This PR introduces a robust, comprehensive solution to...").

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

### Motivation

Flaked ~1 in 5 runs on main, blocking merges for unrelated PRs.

### Describe how you validated your changes

Ran the test 20x locally with the fix; no failures (previously failed ~4/20).
EOF
)"
```

Note there is no "Additional Notes" section here — it was omitted because there was nothing beyond the two sections above worth adding, per the "no filler" guidance.

## Usage

- `/create-pr` — creates a draft PR (default)
- `/create-pr --real` — creates a non-draft PR
- `/create-pr --real team/my-team` — non-draft PR with an extra label

## Output

If you are not following the PR status (step 13): Return the PR URL when done.
Otherwise, defer to `/follow-pr`.
