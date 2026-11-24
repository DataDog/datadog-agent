---
name: resolve-sync-conflict
description: >-
  Resolve a merge conflict while importing public DataDog/datadog-agent main into
  internal ddoghq/datadog-agent main, when the hourly sync workflow failed and
  disabled itself. Prepares and verifies the resolution for a human to publish.
---

# Resolve an internal main sync conflict

**Ownership:** maintained by `@DataDog/agent-devx`.

Internal `main` is a stable integration branch. The hourly workflow merges public
`main` without rebasing or force-pushing existing internal history. On a conflict
it leaves remote main unchanged, disables itself, and alerts `#agent-devx-ops`.

Prepare and verify the resolution; leave publication to the human. Do not run
`--continue` without `--dry-run`, push, or enable the workflow yourself.

## 1. Start a fresh merge

Use a `ddoghq/datadog-agent` clone, where the sync script and workflow live;
this public source repository only carries the shared skill. First check:

```bash
tools/ci/sync-public-main.sh --help
```

The help must describe **merging**, not rebasing. If it still describes a rebase,
stop: the merge-based sync implementation must land internally before using this
procedure. Then run:

```bash
tools/ci/sync-public-main.sh --manual
```

Work in the printed worktree. Do not reuse a worktree prepared by the former
rebase workflow: keep any valuable resolution notes, then start a fresh merge.
The script stores the original and upstream SHAs in worktree-specific state.

## 2. Understand both sides

```bash
base=$(git merge-base refs/sync-public-main/origin refs/sync-public-main/upstream)
git diff "$base" refs/sync-public-main/origin -- <file>
git diff "$base" refs/sync-public-main/upstream -- <file>
```

During this merge, **ours is internal** and **theirs is public upstream**.
Resolve hunk by hunk, preserving internal adaptations and upstream changes.
Do not replace whole files with either side unless you have checked all changes.
Retain internal CI identities and the public Go module/import paths. If upstream
removed or moved edited code, follow the new structure and explain the resolution.

The script regenerates CODEOWNERS from upstream with the internal team/handle
mapping only when the old internal file exactly matches that rewrite at the
merge base. It also rewrites clean upstream additions. If internal CODEOWNERS
has custom rules, resolve it manually and preserve them. If a new personal handle
has no mapping, ask for its internal account and update `CODEOWNERS_RULE` before
regenerating; do not silently publish public personal handles.

Scan newly imported CI configuration for new public repository identity references
that Git merged cleanly. Retain public Go imports, documentation links, dependency
repositories, and explicit public-source remotes. Explain any additional internal
identity rewrites in the hand-over.

## 3. Finish the merge

```bash
git diff --check
git add <resolved-files>       # or git rm for accepted deletions
git merge --continue
```

To abandon this attempt: `git merge --abort`. Never rebase, reset, squash, or
force-push internal main to resolve the sync.

## 4. Verify

```bash
tools/ci/sync-public-main.sh --continue --dry-run
git show --stat HEAD
git diff refs/sync-public-main/origin HEAD   # changes introduced into internal main
```

The dry run checks that both the original internal tip and the captured public
tip remain ancestors and that the working tree is clean. Run checks appropriate
to the resolved files. Report each resolution and any additional identity rewrites.

## 5. Hand over publication

Provide these commands to the human, leaving the worktree in place:

```bash
tools/ci/sync-public-main.sh --continue --workdir <worktree>
git worktree remove <worktree>
```

The first command performs a normal push and re-enables the workflow. If internal
main advanced meanwhile and the push is rejected, start a fresh `--manual` merge
from the updated main and reapply the reviewed resolution. Never force the push.
