---
name: describe-functional-changes
description: >-
  Describe what a code change does functionally (the scenarios whose behavior changes, the new and removed
  behaviors, and what deliberately stays the same) as a short "Functional changes" block for a PR description.
  /create-pr calls it for every PR. Use it directly when asked "what does this PR change functionally",
  "describe the behavior change", "write a functional diff", "explain this change without the code",
  or to improve a PR description that only restates the diff.
allowed-tools: Bash, Read, Glob, Grep
argument-hint: "[<base>...<head> | <PR number>] [context...]"
model: sonnet
---

# Describe functional changes

Turn a diff into a short description of how behavior changes, written for a reviewer who hasn't read the code yet.

AI-written PR descriptions tend to restate the diff: which functions moved, which files changed. From that, a reviewer can't tell what the change *does* without reading every line. This skill writes the missing piece: the scenarios whose outcome is different after the change, phrased so that someone who knows the product but not this code can check them. It complements the technical description of the PR; it doesn't replace it.

**Owning team:** `@DataDog/agent-devx`

**Prerequisites:** a git checkout where the base branch (`main` by default) is available locally. Only a PR number input needs `gh`, authenticated with access to `DataDog/datadog-agent`.

## Inputs

- **A diff range**, the current branch against `main` by default. For `<base>...<head>`, use it instead of `main...HEAD` in the commands below, and `<base>..<head>` for `git log`.
- **Or a PR number**: use `gh pr diff <number>` and `gh pr view <number> --json title,body,baseRefName,headRefName`.
- **Optional context**: the motivation, a linked issue, or what the user said about the change. Commit messages and an existing PR body are context too. Use context to tell which behaviors matter, not as facts about the code: check the claims it makes (e.g. "the Helm chart already mounts X") or mark them `(not verified)`. The description must match what the code does, even when that differs from the stated intent.

## Process

1. **Collect the change.**
   ```bash
   git log --format='%h %s%n%b' main..HEAD
   git diff --stat main...HEAD
   git diff main...HEAD
   ```
2. **Identify who is affected.** Every change has an audience whose experience changes: Agent users and their configuration, the payloads sent to Datadog (metrics, tags, logs, traces), CLI, status, or flare output, other Agent components, developers running invoke tasks or tests, CI pipelines, or agents following a skill. Functional changes are what *they* would observe, so name them before writing anything.
3. **Work out behavior, not edits.** For each meaningful hunk, ask in which situation the outcome is now different. Read the surrounding code, callers, config defaults, and build tags: a diff shows which lines changed, not which paths reach them or on which platforms. Added or changed tests are usually the best evidence of intent: their setup is the scenario and their assertions are the outcome. Sort what you find into the categories below, which are for your analysis, not headings in the output:
   - **Changed**: an existing scenario with a different outcome. Give both sides ("now X, instead of Y") so the reviewer can judge whether the old behavior was relied on.
   - **New**: a scenario that wasn't handled or possible before.
   - **Removed**: something that no longer happens or is no longer possible.
   - **Unchanged**: something a reviewer would reasonably expect to change but doesn't (defaults, existing configs, other platforms, payload formats). This is what lets a reviewer stop worrying about a whole area, so only include the ones a reviewer would actually ask about.
4. **Check each claim.** Every bullet should be backed by code or a test you read. When a behavior depends on something you can't see (runtime config, another repository, the backend), read more or mark the bullet `(not verified)`, rather than presenting a guess as a fact. If only part of a bullet is checked, split it so each bullet is either checked or marked. Drop bullets that restate the code without saying what it changes for anyone. If you notice a likely bug or an unhandled edge case, keep it for the notes (see "Output"): the block describes what the code does, not what it should do.
5. **Write the block** in the format below.

## Output

Return a Markdown block as raw text (not in a code fence), ready to paste under "What does this PR do?" after the technical summary. The placeholders show the shape, not required wording:

```markdown
**Functional changes**
- <Who or what> now <new outcome> when <scenario>, instead of <old outcome>.
- <Scenario that is now handled or possible>.
- <Thing that no longer happens>.
- <Area a reviewer might worry about> is unchanged.
```

What makes a good block:

- **One scenario per bullet, seen from outside, in one or two sentences.** Name the affected audience when it isn't obvious (e.g. "In `security-agent status`, …"). Config keys, CLI flags, metric and tag names, labels, files, and commands are fine to name. Internal function and type names usually aren't: a bullet that needs one is probably describing the implementation. Leave out how you verified the claim; reviewers have the diff for that.
- **Most important first.** Aim for 2 to 8 bullets, fewer for small changes. If you need more, the PR probably does several things; say so rather than listing everything. Unchanged bullets only qualify the changes, so keep them to one or two.
- **Group distinct concerns.** When a change affects separate areas (e.g. behavior and security, or two components), give each its own bold label, like `**Security changes**`, instead of one long list.
- **Before/after only when it helps.** When the scenario is new or the old behavior is obvious, state the new behavior alone.
- **Plain tone**, like the rest of the description: no "This PR introduces…", no adjectives selling the change.
- **No functional change?** Return one line instead of a list: `No functional change: <what was restructured>; <what shows the behavior is preserved>.` For a refactor, that is the main thing a reviewer needs to know.

After the block, add short notes for the user, outside the block and only when there is something to say: what would confirm each `(not verified)` bullet, likely bugs or edge cases you noticed, and changed behavior that no test covers. They don't go in the PR description as is, but the user may want to act on them before opening the PR.

## Examples

**CI workflow change**, condensed from [#58985](https://github.com/DataDog/datadog-agent/pull/58985). Its technical summary was "Merges the three PR labeling workflows into one, and moves the community PR Slack notification to its own workflow." The functional block then says what that changes for PR authors and maintainers:

```markdown
**Functional changes**
- Every PR gets `internal` (branch in this repo) or `community` (fork), whatever its target branch, including PRs with merge conflicts and fork PRs on every push.
- Team labels are computed from `main`'s CODEOWNERS for every PR (previously the PR's own version for internal PRs).
- The community Slack notification is sent whenever `community` is added (by the labeler or by hand), instead of only when a fork PR is opened.
- The stale bot's closing comment says 30 days instead of 15. Its selection rules are unchanged.

**Security changes**
- No code from the PR runs with the labeling token anymore (internal PRs used to run their own `tasks/`).
```

Workflow file names, job names, and the YAML restructuring are absent: the summary sentence and the diff already cover them.

**Agent change** (hypothetical). A bullet that describes the implementation:

```markdown
- Add a `handleTruncation` method to the tailer and call it from `readForever`.
```

The same change, described functionally:

```markdown
**Functional changes**
- When a tailed file is truncated in place (copytruncate rotation), the Agent now resumes from the start of the file, instead of waiting for it to grow past the previous offset and losing the lines in between.
- Files rotated by rename and `logs_config` defaults are unchanged.
```

**Refactor**:

```markdown
No functional change: config parsing for the check moves into its own package; the existing parsing unit tests pass unmodified.
```
