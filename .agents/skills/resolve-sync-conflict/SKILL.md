---
name: resolve-sync-conflict
description: >-
  Resolve a rebase conflict of the internal ddoghq/datadog-agent main onto the public
  DataDog/datadog-agent main, when the hourly "Rebase internal main onto public main" workflow
  failed and disabled itself. Prepares and verifies the resolution; a human with admin rights pushes it.
  Trigger phrases include:
  - "the internal main sync has a conflict"
  - "resolve the sync-public-main conflict"
  - "Internal main sync hit a rebase conflict" (Slack alert from #agent-devx-ops)
---

# Resolve an internal main sync conflict

**Ownership:** maintained by `@DataDog/agent-devx`.

Internal `main` of ddoghq/datadog-agent is public `main` plus an internal patch stack, rebased
hourly by `.github/workflows/sync-public-main.yml` through `tools/ci/sync-public-main.sh`.
On a conflict, the workflow leaves internal main unchanged, disables itself and posts in
`#agent-devx-ops`. The resolved stack becomes the new internal main, so the same conflict
does not come back unless upstream touches the same lines again.

**You prepare the resolution; you never push it.** The push rewrites a protected branch and
needs a human with admin rights: stop at step 5 and hand over.

## 1. Start the rebase

From a ddoghq/datadog-agent clone whose `origin` points to ddoghq (reused for fetch credentials):

```bash
tools/ci/sync-public-main.sh --manual
```

It creates a worktree, fetches both mains under `refs/sync-public-main/` and stops on the conflict,
printing the worktree path and the conflicting files. Work in that worktree from now on.

## 2. Understand what the internal patch wanted

Find the commit being replayed (`git status` names it) and read its change to each conflicting
file, and what upstream changed meanwhile:

```bash
git show <internal-commit> -- <file>                                       # internal intent
git log --oneline <internal-commit>^..refs/sync-public-main/upstream -- <file>  # upstream changes
```

## 3. Resolve each file

Re-apply the internal intent on top of the upstream version; never revert upstream changes.
Known patterns of the internal patch:

| Upstream change | Resolution |
|---|---|
| Content changed in a file the patch rewrites from `DataDog/…` to `ddoghq/…` (`.github/CODEOWNERS`, chainguard `*.sts.yaml` subjects and `job_workflow_ref`, CI scripts) | Take the upstream side (`git checkout --ours <file>`: during a rebase *ours* is upstream, *theirs* the internal commit), then re-apply the same rewrite to the new lines. For chainguard subjects keep the internal `repo:ddoghq@<org-id>/datadog-agent@<repo-id>:…` form used by the other internal `.sts.yaml` files. |
| File deleted upstream, modified internally (`modify/delete`) | Usually accept the deletion (`git rm <file>`), then remove internal references to it (grep the old name, e.g. in chainguard `job_workflow_ref` patterns). Keep the file only if the internal change is the reason it exists; say so in the hand-over. |
| Internal-only addition (lines with no public counterpart, e.g. `git remote set-url origin "$AIX_GIT_REMOTE"`) | Keep it, placed where it still makes sense in the new upstream code. |

Anything else: stop and ask rather than guess.

Check that no marker is left and that the rewrite did not miss a spot:

```bash
git diff --check
git grep -n 'DataDog/datadog-agent' -- <resolved files>   # review every hit
```

## 4. Finish the rebase

```bash
git add <files>        # or git rm
git rebase --continue  # repeat steps 2-4 if another commit conflicts
```

## 5. Verify, then hand over

```bash
tools/ci/sync-public-main.sh --continue --dry-run
git range-diff refs/sync-public-main/upstream refs/sync-public-main/origin HEAD
```

The range-diff compares the internal stack before and after: the only differences should be your
resolutions. Report to the user, without pushing:
- each conflicting file and how you resolved it (and any file you kept despite an upstream deletion),
- the range-diff summary,
- the commands they must run themselves: the first pushes with a lease and re-enables the workflow,
  the second removes the worktree once the push succeeded:

```bash
tools/ci/sync-public-main.sh --continue --workdir <worktree>
git worktree remove <worktree>
```

Leave the worktree in place: the push needs it.
