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

The script checks **every exported commit**, not only the final diff: a reference or file
added by one commit and removed by a later one is gone from the result but would still be
public in the history. So a fix has to rewrite the commit that introduced the problem, not
add a new commit on top.

| Finding | What to do |
|---|---|
| Internal-only file changed | Usually an internal adaptation leaked: drop that part of the change. If the file must change publicly (e.g. a real CODEOWNERS update), show the diff to the user; only they decide to re-run with `--allow-path '<glob>'`. |
| Lines added by a commit reference internal repos or accounts | Rewrite them with public equivalents, or remove them, in that commit. For the last commit, edit and `git commit --amend --no-edit`. For an older one, either fix it on the private branch and start step 2 again, or, if the user agrees to lose the commit split, squash the export into one commit: `git reset --soft refs/export-to-public/upstream`, fix, then commit. |
| Commit message references internal repos | Propose a new message to the user. Apply it with `git commit --amend` on the last commit; for an older one, reword it on the private branch and start step 2 again. |
| Commit identity uses an internal account | See "Identities" below. |
| Internal main commit / merge commit | Should not happen with the script; start over from step 2 and report it. |

To start step 2 again, first run the cleanup command printed by the script (it removes the
worktree and its `export-to-public/<name>` branch). Then run `--check` until it prints
`Ready to export`.

### Identities

The **committer** is whoever runs the cherry-picks: an internal committer means the clone's
`user.name` or `user.email` is set to an internal identity. Ask the user to fix it
(`git config user.name`, `git config user.email`), then re-create the commits so they pick it
up:

```bash
git rebase --exec 'git commit --amend --no-edit' refs/export-to-public/upstream
```

The **author** is kept from the private commits, and `git commit --amend` doesn't change it.
Fix it only for the user's own commits, with the identity they confirm for public use (by
default the `user.name` and `user.email` of their public clone):

```bash
git commit --amend --no-edit --author='Jane Doe <jane.doe@datadoghq.com>'   # last commit
git rebase --exec "git commit --amend --no-edit --author='Jane Doe <jane.doe@datadoghq.com>'" \
  refs/export-to-public/upstream                                             # every commit
```

If some commits are someone else's, don't rewrite them with the user's identity: ask the
user for that person's public name and email, or leave the commits out.

### Review each commit

Then review what the script can't judge, **commit by commit** (`git log -p
refs/export-to-public/upstream..HEAD`), not only the final diff: no private PR, Slack thread,
document or internal-only file in any patch or message, and no more detail about a
vulnerability than the fix requires.

## 4. Hand over

End with a message in this shape, filled in from the script output and your review:

```markdown
Public branch `<name>` is ready in `<worktree>`, based on public main `<sha>`. Nothing is pushed.

Exported commits (author, subject):
- `<sha>` <author>: <subject>

Checks: `export-to-public.sh --check` passes. Per-commit review: <done, and what was fixed>.
Allowed paths: <none, or each `--allow-path` the user approved>.
Signature: <public key / `ddoghq` key: may show as Unverified>.
Embargo: <not a security fix / security fix: wait for #agent-release-sync before pushing>.

To publish, from `<worktree>`, with your public gh account:
    git push https://github.com/DataDog/datadog-agent.git HEAD:refs/heads/<name>
    gh pr create --repo DataDog/datadog-agent --head <name> --draft
Then clean up: <cleanup command printed by the script>

Draft PR description:
<description following .github/PULL_REQUEST_TEMPLATE.md, with no link to the private PR>

The public PR runs its own full CI and needs reviews from the public CODEOWNERS: results from
the private PR don't carry over.
```

The `/create-pr` skill can also produce the description once the branch is pushed.

Once the public PR merges, the hourly sync brings it into internal `main`; the private PR
can then be closed.
