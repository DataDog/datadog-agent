---
name: describe-functional-changes
description: >-
  Describe what a code change does functionally (the scenarios whose behavior changes, the new and removed
  behaviors, and what deliberately stays the same) as a short "Functional changes" Before | After table for a
  PR description.
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
- **Optional context**: the motivation, a linked issue, or what the user said about the change. Commit messages and an existing PR body are context too. Use context to tell which behaviors matter, not as facts about the code: check the claims it makes (e.g. "the Helm chart already mounts X") before repeating them. The description must match what the code does, even when that differs from the stated intent.

## Process

1. **Collect the change.**
   ```bash
   git log --format='%h %s%n%b' main..HEAD
   git diff --stat main...HEAD
   git diff main...HEAD
   ```
2. **Identify who is affected.** Every change has an audience whose experience changes: Agent users and their configuration, the payloads sent to Datadog (metrics, tags, logs, traces), CLI, status, or flare output, other Agent components, developers running invoke tasks or tests, CI pipelines, or agents following a skill. Functional changes are what *they* would observe, so name them before writing anything.
3. **Work out behavior, not edits.** For each meaningful hunk, ask in which situation the outcome is now different. Read the surrounding code, callers, config defaults, and build tags: a diff shows which lines changed, not which paths reach them or on which platforms. Added or changed tests are usually the best evidence of intent: their setup is the scenario and their assertions are the outcome. Sort what you find into the categories below, which are for your analysis, not headings in the output:
   - **Changed**: an existing scenario with a different outcome. Note both sides: they become the Before and After of a row, and let the reviewer judge whether the old behavior was relied on.
   - **New**: a scenario that wasn't handled or possible before.
   - **Removed**: something that no longer happens or is no longer possible.
   - **Unchanged**: something a reviewer would reasonably expect to change but doesn't (defaults, existing configs, other platforms, payload formats). This is what lets a reviewer stop worrying about a whole area, so only include the ones a reviewer would actually ask about.
4. **Check each claim.** Every claim should be backed by code or a test you read. For docs and skills, that includes what they say about the repo (paths, commands, files): a wrong one is a likely bug. When a behavior depends on something you can't see (runtime config, another repository, the backend), read more, or leave it out of the block and list it in the notes, rather than presenting a guess as a fact. Drop anything that restates the code without saying what it changes for anyone. If you notice a likely bug or an unhandled edge case, keep it for the notes (see "Output"): the block describes what the code does, not what it should do.
5. **Write the block** in the format below.

## Output

Return a Markdown block as raw text (not in a code fence), ready to paste under "What does this PR do?" after the technical summary. The placeholders show the shape, not required wording:

```markdown
**Functional changes**

| Before | After |
| --- | --- |
| <Scenario>: <old outcome> | <New outcome> |
| <Something that used to happen> | <It no longer happens> |
| — | <Scenario that is now handled or possible> |

Unchanged: <area a reviewer might worry about>.
```

What makes a good block:

- **Show before and after.** Most changes modify something that already exists, and seeing the old and new behavior side by side is what lets a reviewer judge whether the change is right, so lead with the table. Start the Before cell with the scenario in a few words (e.g. "`HOST_ROOT` mounted: …"), so that each row reads on its own; a short After cell is fine.
- **Don't invent a before.** For a scenario that wasn't handled or possible, put `—` in Before. When nothing existed before at all (a new check, command, or skill), skip the table and list as bullets what it enables and its notable effects, not each of its steps.
- **One scenario per row, seen from outside.** Table cells can't hold line breaks, so keep each cell to a short sentence. Name the affected audience when it isn't obvious (e.g. "In `security-agent status`, …"). Config keys, CLI flags, metric and tag names, labels, files, and commands are fine to name. Internal function and type names usually aren't: a cell that needs one is probably describing the implementation. Leave out how you verified the claim; reviewers have the diff for that.
- **Most important first.** Aim for at most 8 rows; one is fine for a small change. If you need more, the PR probably does several things; say so rather than listing everything.
- **Unchanged goes last**, after the table or bullets, as one or two short lines: they only qualify the changes.
- **Group distinct concerns.** When a change affects separate areas (e.g. behavior and security, or two components), give each its own bold label and table, like `**Security changes**`, instead of one long table.
- **Plain tone**, like the rest of the description: no "This PR introduces…", no adjectives selling the change.
- **No functional change?** Return one line instead of a table: `No functional change: <what was restructured>; <what shows the behavior is preserved>.` For a refactor, that is the main thing a reviewer needs to know.

After the block, add short notes for the user, outside the block and only when there is something to say: claims you couldn't verify and what would confirm them, likely bugs or edge cases you noticed, and changed behavior that no test covers. They don't go in the PR description as is, but the user may want to act on them before opening the PR.

## Examples

**CI workflow change**, reshaped from [#58985](https://github.com/DataDog/datadog-agent/pull/58985). Its technical summary was "Merges the three PR labeling workflows into one, and moves the community PR Slack notification to its own workflow." The functional block then says what that changes for PR authors and maintainers:

```markdown
**Functional changes**

| Before | After |
| --- | --- |
| Some PRs never got `internal` or `community`, e.g. PRs with merge conflicts, for which GitHub skips `pull_request` workflows | Every PR gets `internal` (branch in this repo) or `community` (fork), whatever its target branch |
| Team labels of internal PRs came from the PR's own CODEOWNERS | Team labels come from `main`'s CODEOWNERS for every PR |
| The community Slack notification was sent only when a fork PR was opened | It is sent whenever `community` is added, by the labeler or by hand |
| The stale bot's closing comment said 15 days | It says 30 days |

Unchanged: the stale bot's selection rules.

**Security changes**

| Before | After |
| --- | --- |
| Internal PRs ran their own `tasks/` code with the labeling token | No code from the PR runs with the labeling token |
```

Workflow file names, job names, and the YAML restructuring are absent: the summary sentence and the diff already cover them.

**Agent change** (hypothetical). A description of the implementation:

```markdown
- Add a `handleTruncation` method to the tailer and call it from `readForever`.
```

The same change, described functionally:

```markdown
**Functional changes**

| Before | After |
| --- | --- |
| When a tailed file was truncated in place (copytruncate rotation), the Agent waited for it to grow past the previous offset, losing the lines in between | The Agent resumes from the start of the truncated file |

Unchanged: files rotated by rename, and `logs_config` defaults.
```

**Nothing existed before** (hypothetical new check), so bullets instead of a table:

```markdown
**Functional changes**
- The new `foo` check, disabled by default, reports `foo.connections.active` and `foo.errors` every 15 seconds once configured.
- `agent status` lists it under Collector like other checks.
```

**Refactor**:

```markdown
No functional change: config parsing for the check moves into its own package; the existing parsing unit tests pass unmodified.
```
