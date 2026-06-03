# Workspace S2: Drop `bits` Pre-Bake (group-ownership)

## Problem

The `dev-env-workspace` buildimage (post-PR #1089 in `datadog-agent-buildimages`) pre-bakes the
`bits` user (UID 2000) so toolchains owned `root:build-shared` are writable at runtime. The
mandatory Datadog Workspace `base` devcontainer feature unconditionally runs
`useradd --uid 2000 bits` at provision time. This collision (`useradd: user 'bits' already
exists`) breaks workspace provisioning.

## Solution

Remove the `bits` user pre-bake from the buildimage entirely. Let the `base` feature create `bits`
at runtime as it was always designed to do. Restore toolchain write access via `build-shared` group
membership, granted by the agent's own devcontainer feature after the base feature runs.

**Why this works:** every baked toolchain is already `root:build-shared` with setgid directories
(`fix-shared-perms.sh`). The existing `dd-build` and `dd` users access all toolchains (RVM, Rust,
Go, Conda) via `build-shared` membership alone — never via user ownership. `bits` is identical.

## Changes

### `datadog-agent-buildimages` — `dev-envs/linux/variants/workspaces/setup.sh`
Removed the `bits` user pre-bake block:
- `useradd ... --groups users,build-shared,sudo bits`
- `if getent group docker; then usermod -a -G docker bits; fi`
- `seed_home "${bits_home}" bits dog`
- `install -d -m 700 -o bits -g dog "${bits_home}/.ssh"`
- `passwd -d bits` / `usermod -U bits`

Kept everything unrelated to `bits` (`dog` user, sudoers, zshenv). `build-shared` (GID 9001) is
created in `linux/Dockerfile:371` and survives — not touched here.

### `datadog-agent` — `.devcontainer/datadog/default/features/datadog-agent/install.sh`
Replaced the unguarded `usermod -aG docker bits` with guarded forms for both groups:
```sh
getent group docker >/dev/null && usermod -aG docker bits
getent group build-shared >/dev/null && usermod -aG build-shared bits
```
The `getent` guard makes this a clean no-op on stock workspace images where these buildimage-only
groups are absent (confirmed live on a stock workspace: exit 0, no change).

## Tested (2026-06-03, image `v116490909-fc35a1a5`)

| Test | Result |
|---|---|
| Collision proof: `useradd bits` on post-#1089 image | `useradd: user 'bits' already exists` ✅ |
| File A: `userdel -r bits` removes pre-baked user | `bits removed ✓` ✅ |
| Base feature `useradd` succeeds after de-bake | `useradd bits succeeded (no collision) ✓` ✅ |
| File B: `usermod -aG build-shared bits` adds membership | `bits in build-shared ✓` ✅ |
| RVM gem write as bits (fresh exec, login shell) | `GEM_HOME=/opt/dd/rvm/gems/ruby-2.7.2` write PASS ✅ |
| Rustup dir write as bits (`/opt/dd/rustup`) | PASS ✅ |
| Go build as bits (`GOPATH=/var/config/dd/go`) | PASS ✅ |
| Stock-workspace guard: no-op when `build-shared` absent | exit 0 ✅ |

## Adoption (next steps)

1. This PR (File B) is already merged — it is a **no-op** on the current pre-#1089 pinned image.
2. Merge `datadog-agent-buildimages` PR (File A) → new image tag published.
3. In `datadog-agent`: bump `prebuild-devcontainer.json` to new tag → regenerate composite →
   update `devcontainer.json` digest.

**Never bump the image tag before File B is merged.** See
`.claude/plans/workspace-s2-group-ownership.md` for the full ordering invariant and acceptance
gate (orchestrator provision must complete green before adoption step 3).

## Scope

Fixes P1 (collision) only. Does not address P2 (base feature installing its managed tools onto the
buildimage — a separate stewardship question). See `workspace-solution-options.md` for the full
solution comparison including the S4 split-feature path (P2-complete but higher cost).
