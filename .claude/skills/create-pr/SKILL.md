---
name: create-pr
description: Create a pull request for the current branch with proper labels and description
disable-model-invocation: true
allowed-tools: Bash, Read, Glob
argument-hint: "[--real] [additional labels...]"
---

Create a pull request for the current branch following the Datadog Agent contributing guidelines.

## Instructions

1. **Check the current branch** and ensure it's not `main`
2. **Get the commits** on this branch compared to `main` using `git log main..HEAD`
3. **Get the diff** using `git diff main..HEAD` to understand all changes
4. **Read the PR template** from `.github/PULL_REQUEST_TEMPLATE.md`
5. **Push the branch** to origin if needed
6. **Open the PR**: By default, open as **Draft** using `gh pr create --draft`. If `$ARGUMENTS` contains `--real`, open as a regular (non-draft) PR instead (omit the `--draft` flag). Remove `--real` from `$ARGUMENTS` before processing remaining arguments as labels.
7. **PR title**: Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) format, prefixed with the general area of change. Examples:
   - `fix(e2e): Fix flaky diagnose test`
   - `feat(logs): Add new log pipeline`
   - `refactor(config): Simplify endpoint resolution`
8. **Labels**: Choose appropriate labels (plus any additional labels passed as $ARGUMENTS):
   - If the PR only changes tests, docs, CI config, or developer tooling (no Agent binary code changes), use `changelog/no-changelog` and `qa/no-code-change`
   - If the PR changes Agent binary code and QA was done, use `qa/done`
   - If the PR changes Agent binary code, a reno release note is expected (remind the user)
   - Add `backport/<branch-name>` if the user asks for a backport
9. **PR body**: Fill in the PR template sections:
   - **What does this PR do?**: A clear description of what is changed. Must be readable independently, tying back to the changed code.
   - **Motivation**: A reason why the change is made. Point to an issue if applicable. Include drawbacks or tradeoffs if any.
   - **Describe how you validated your changes**: How you validated the change (tests added/run, benchmarks, manual testing). Only needed when testing included work not covered by test suites.
   - **Additional Notes**: Any extra context, links to predecessor PRs if part of a chain, notes that make code understanding easier.

## PR Description Guidelines (from CONTRIBUTING.md)

The PR description should incorporate everything reviewers and future maintainers need:
- A description of what is changed
- A reason why the change is made (pointing to an issue is a good reason)
- When testing had to include work not covered by test suites, a description of how you validated your change
- Any relevant benchmarks
- Additional notes that make code understanding easier
- If part of a chain of PRs, point to the predecessors
- If there are drawbacks or tradeoffs, raise them

## Example

```bash
gh pr create --draft \
  --title "fix(e2e): Fix flaky diagnose test by adding missing fakeintake redirect" \
  --label "changelog/no-changelog" \
  --label "qa/no-code-change" \
  --body "$(cat <<'EOF'
### What does this PR do?

<description of changes>

### Motivation

<why this change is needed>

### Describe how you validated your changes

<testing done>

### Additional Notes

<any extra context>
EOF
)"
```

## Usage

- `/create-pr` — creates a draft PR (default)
- `/create-pr --real` — creates a non-draft PR
- `/create-pr --real team/my-team` — non-draft PR with an extra label

## Output

Return the PR URL when done.
