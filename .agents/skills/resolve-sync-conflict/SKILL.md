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
base=$(git merge-base refs/sync-public-main/origin refs/sync-public-main/upstream)
git show <internal-commit> -- <file>                                 # internal intent
git log --oneline "$base"..refs/sync-public-main/upstream -- <file>  # upstream changes
```

## 3. Resolve each file

Re-apply the internal intent on top of the upstream version; never revert upstream changes.
Resolve **hunk by hunk**, keeping the upstream side of each conflict and re-applying the internal
change in it. Avoid `git checkout --ours <file>` (during a rebase *ours* is upstream): it takes the
whole upstream file and silently drops the internal changes git merged cleanly elsewhere in it.

Known patterns of the internal patch:

| Upstream change | Resolution |
|---|---|
| `.github/CODEOWNERS` conflicts | Don't resolve hunks: regenerate it from upstream with the internal rewrite (see below). |
| Content changed in a file the patch rewrites from `DataDog/…` to `ddoghq/…` (chainguard `*.sts.yaml` subjects and `job_workflow_ref`, `dd-octo-sts --scope`, CI scripts, `GithubAPI(...)` repos) | Keep the upstream side of the hunk, then re-apply the same rewrite to its lines. For chainguard subjects keep the internal `repo:ddoghq@<org-id>/datadog-agent@<repo-id>:…` form used by the other internal `.sts.yaml` files. |
| File deleted upstream, modified internally (`modify/delete`) | Usually accept the deletion (`git rm <file>`), then remove internal references to it (grep the old name, e.g. in chainguard `job_workflow_ref` patterns). Keep the file only if the internal change is the reason it exists; say so in the hand-over. |
| Internal-only addition (lines with no public counterpart, e.g. `git remote set-url origin "$AIX_GIT_REMOTE"`) | If upstream adopted it, keep the upstream version only. Otherwise keep it, placed where it still makes sense in the new upstream code. |
| Code moved or removed upstream while the internal patch edited it | Follow it: re-apply the internal change where the code went (see step 3b), or drop it with the removed code. |

Anything else: stop and ask rather than guess.

### CODEOWNERS

The internal file is the public one with GitHub teams moved to the `ddoghq` org (in both `@DataDog/`
and `@Datadog/` spellings) and personal handles mapped to their `ddoghq` accounts. Start from the team
rule and check what else the current internal file maps, by rebuilding it from the public one at
the merge base:

```bash
rule='s#@[Dd]ata[Dd]og/#@ddoghq/#g'
diff <(git show "$base":.github/CODEOWNERS | sed -E "$rule") <(git show refs/sync-public-main/origin:.github/CODEOWNERS)
```

Each remaining difference should be a personal handle mapped to its `ddoghq` account: add one
`; s#@<public-handle>([[:space:]]|$)#@<ddoghq-handle>\1#g` per mapping to `rule` until the diff is
empty. Stop and ask if a difference is anything else. A handle new upstream has no mapping yet: ask
the user for its `ddoghq` account. Then regenerate:

```bash
git show refs/sync-public-main/upstream:.github/CODEOWNERS | sed -E "$rule" > .github/CODEOWNERS
grep -oE '@[A-Za-z0-9_-]+([[:space:]]|$)' .github/CODEOWNERS | sort -u   # personal handles: all must be ddoghq accounts
```

### 3b. Look beyond the conflicts

Upstream can add public-identity references that conflict with nothing: new files, code moved to
a new file, new functions next to rewritten ones. Git merges them silently and they break internal
CI later. Scan what upstream added to CI configuration since the merge base:

```bash
git diff "$base" refs/sync-public-main/upstream -U0 -- .github .gitlab tools/ci tasks test \
  | awk '/^\+\+\+ b\//{f=substr($0,7)} /^\+[^+]/ && /DataDog\/datadog-agent|"org": *"DataDog"|repo:DataDog|--scope DataDog/ \
         && !/github\.com\/DataDog\/datadog-agent\// {print f": "$0}'   # skip Go imports and web links
```

Rewrite a hit when the internal patch already rewrites the same pattern elsewhere (compare with
`git grep` on `refs/sync-public-main/origin`): `dd-octo-sts --scope`, `self.*` chainguard policies,
git remotes such as `AIX_GIT_REMOTE`, `GithubAPI(...)` repositories, PR-comment payload orgs. Leave
comments, documentation links, test fixtures and Go import paths as they are. List every rewrite in
the hand-over: the user did not see them as conflicts.

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
- each rewrite made outside the conflicts (step 3b), with why, and the hits you left as they are,
- the range-diff summary,
- the commands they must run themselves: the first pushes with a lease and re-enables the workflow,
  the second removes the worktree once the push succeeded:

```bash
tools/ci/sync-public-main.sh --continue --workdir <worktree>
git worktree remove <worktree>
```

Leave the worktree in place: the push needs it.
