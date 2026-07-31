---
name: cws-btfhub-sync
description: Sync CWS BTFHub constants after adding a new constantfetch offset, so pre-BTF kernels can resolve it. Use when KMT secagent jobs log "failed to fetch constant for <name>".
allowed-tools: Read, Grep, Bash, AskUserQuestion
model: sonnet
---

# Syncing CWS BTFHub constants

A new `appendOffsetofRequest` in `probe_ebpf.go` resolves via BTF on modern kernels, but
pre-BTF kernels read the offset from the checked-in, arch-split constants:

- `pkg/security/probe/constantfetch/constants_amd64.json`
- `pkg/security/probe/constantfetch/constants_arm64.json`

These predate your change and so lack the new entry. Regenerating them is the fix.

## Run it

```bash
gh workflow run cws-btfhub-sync.yml --ref main -f base_branch=<your-feature-branch>
```

**The trap:** `--ref` selects the workflow *definition* — keep it `main`. `base_branch` is the
ref the generate and combine jobs check out, **and** the base of the PR it opens. Leave
`base_branch` at its `main` default and the generator never sees your new offset requests: the
sync yields an empty diff and it looks like btfhub simply lacks the offsets.

Before running, confirm the failing distros are covered by the `cone` matrix in
`.github/workflows/cws-btfhub-sync.yml`.

## Wait for it

It is a ~15-job matrix and takes tens of minutes. Poll it from a **background** shell so the
wait costs nothing and you get re-invoked on exit:

```bash
until [ "$(gh run view <run-id> --json status -q .status)" = completed ]; do sleep 60; done
gh run view <run-id> --json conclusion -q .conclusion
```

Do not run `gh run watch` in the foreground (it outlives the tool timeout) and do not chain
short `sleep`s in the main loop. When the notification arrives, carry straight on to *Land it*
in the same turn — this is one task, not a hand-off.

## Land it

On success the workflow opens a PR titled `CWS: sync BTFHub constants` from a
`cws/constants-sync-*` branch, targeting your branch:

```bash
gh pr list --search "CWS: sync BTFHub constants" --state open --json number,headRefName,url
```

Cherry-pick its commit onto your branch, push, then **close** the PR. Confirm the picked
commit touches `btfhub/constants.json` and that your new offset names appear in the diff.
