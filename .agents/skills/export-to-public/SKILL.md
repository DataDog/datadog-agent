---
name: export-to-public
description: >-
  Turn a branch or PR of the private ddoghq/datadog-agent repository into a branch for a
  public DataDog/datadog-agent PR: cherry-pick only its own commits onto public main and
  check that no internal commit, file, reference or SHA goes along. Prepares and verifies
  the branch for a human to push. Use for "open this private PR publicly", "export my
  branch to the public repo", "move this fix to DataDog/datadog-agent".
---

# Export a private branch to the public repository

**Ownership:** maintained by `@DataDog/agent-devx`.

**Prerequisites:** Git 2.45 or newer, read access to `ddoghq/datadog-agent`, and for the
human who pushes: write access to `DataDog/datadog-agent`, with `gh` logged in on the public
account (`gh auth switch`; the `_ddog` account can't open PRs there).

Internal `main` is public `main` plus internal-only commits (CI identity, token policies,
sync tooling) and hourly sync merges. A branch made from it carries all of them, so it
can't be pushed as is: only its own commits must be rebuilt on public `main`.

Pushing to `DataDog/datadog-agent` discloses the change at once, even before a PR is
opened, and can't be undone. **Prepare and verify; never push, open the public PR or
pass `--allow-path` yourself** unless the user explicitly asks for that step in this session.

## 1. Confirm what goes public, and when

- Identify the branch: the user's words, else the current branch, else the head branch of
  the private PR they point to (`gh pr view <n> -R ddoghq/datadog-agent --json headRefName`).
- Ask whether the change is a security fix. If it is, the push must wait until the timing is
  agreed with `#agent-release-sync`: prepare the branch, but say so in the hand-over.
- The public branch name is public too. Check it doesn't describe a vulnerability
  (e.g. `fix-cve-...`); pick a neutral one with `--name` if needed.

## 2. Prepare the branch

Run from a clone of the **public** repository when the user has one: the export commits are
then signed with their public signing key, and pushed with their public account. Commits made
in a clone with a `ddoghq` remote are signed with the `ddoghq` key (Datadog's
`git-config-tool` switches keys per remote), which may show as Unverified on the public
repository. Fetch the private branch without adding a remote:

```bash
git fetch https://github.com/ddoghq/datadog-agent.git <branch>:refs/private/<branch>
tools/ci/export-to-public.sh [--name <public-branch>] refs/private/<branch>
```

From a `ddoghq/datadog-agent` clone, run `tools/ci/export-to-public.sh <branch>` instead, and
tell the user in the hand-over that the commits carry the `ddoghq` signature.

It fetches both `main` branches (under `refs/export-to-public/`, remotes untouched), lists
the commits it exports (those of `<branch>` in neither main, merges excluded), and
cherry-picks them, without `-x`, in a new worktree on public `main`. Check that the
listed commits are exactly the user's work; if not, stop and ask.

**On a cherry-pick conflict**, the change touches a file internal commits modified. In the
worktree, resolve towards the public version: keep the user's change, drop internal
adaptations (`ddoghq` names, project ID `14012`, `_ddog` handles). Then:

```bash
git add <files> && git cherry-pick --continue
tools/ci/export-to-public.sh --check --workdir <worktree>
```

## 3. Fix what the checks report

The script blocks on:

| Finding | What to do |
|---|---|
| Internal-only file changed | Usually an internal adaptation leaked: drop that part of the change. If the file must change publicly (e.g. a real CODEOWNERS update), show the diff to the user; only they decide to re-run with `--allow-path '<glob>'`. |
| Added lines reference internal repos or accounts | Rewrite them with public equivalents, or remove them. Amend the commit that added them. |
| Commit message or identity references internal repos | Propose a new message to the user. Apply it with `git commit --amend` on the last commit; for an older one, remove the worktree and its `export-to-public/<name>` branch (cleanup command printed by the script), reword on the private branch, and start step 2 again. |
| Internal main commit / merge commit | Should not happen with the script; start over from step 2 and report it. |

Then run `--check` again until it prints `Ready to export`.

Also review by hand what the script can't judge: the diff and messages must not mention the
vulnerability more than the fix requires, nor link private PRs, Slack threads or documents.

## 4. Hand over

Give the user the commands printed by the script (push to the public repo, open a draft
PR, remove the worktree), plus:

- A draft public PR description following `.github/PULL_REQUEST_TEMPLATE.md`, with no
  reference to the private PR. The `/create-pr` skill can produce it once the branch is pushed.
- A reminder that CI results don't carry over: the public PR runs its own full pipeline and
  needs reviews from the public CODEOWNERS.
- For a security fix, the reminder to agree on the timing with `#agent-release-sync`.

Once the public PR merges, the hourly sync brings it into internal `main`; the private PR
can then be closed.
