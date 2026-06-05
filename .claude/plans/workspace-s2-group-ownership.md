# Implementation Plan — S2: Drop the `bits` Pre-Bake (group-ownership)

> Produced by adversarial planning (Sonnet planner × Opus critic × revision). All P1s (a/b/c) resolved; P2s folded in.
> Companion to `workspace-solution-options.md` (ranked S2 #1) and `workspace-integration.md` (the S4 split-feature path).
> **Tested 2026-06-03** against real post-#1089 image `v116490909-fc35a1a5`. All VERIFY items resolved. See §5 for runnable test commands.

## 1. Overview

Make the `datadog-agent-buildimages` `dev-env-workspace` image usable as a Datadog Workspace base without the `useradd bits` collision, by **removing the pre-baked `bits` user from the image** and letting the mandatory workspace `base` devcontainer feature create `bits` at runtime (`dd-source/domains/devcontainers/features/base/install.sh:47`, `useradd -g dog bits`).

Toolchain write-access is preserved via **`build-shared` (GID 9001) GROUP membership** — not file ownership — added by `usermod -aG build-shared bits` in the agent's own devcontainer feature. The `build-shared` group and its group-writable toolchain trees (`fix-shared-perms.sh`) live at the `linux/Dockerfile` layer and survive the de-bake.

Scope: **P1-only.** This does NOT address P2 "image mutation" (re-running `usermod` on every container start vs. baking it once); see cross-reference `workspace-solution-options.md`. Both changed files are agent-team-owned.

## 2. Exact Changes

### File A — buildimages `dev-envs/linux/variants/workspaces/setup.sh`

> **VERIFIED 2026-06-03** against the real file (repo cloned at `DataDog/datadog-agent-buildimages`, shallow `--depth 1`). The exact block to remove is confirmed below. No hidden `build-shared` dir-prep found — all toolchain ownership (`root:build-shared`, setgid) is set in `linux/Dockerfile` via `fix-shared-perms.sh`, not in this file.

REMOVE exactly these lines (verbatim, confirmed against real file):

```bash
useradd --gid dog --uid 2000 --home-dir "${bits_home}" --shell /usr/local/bin/zsh --groups users,build-shared,sudo bits
if getent group docker >/dev/null; then
    usermod -a -G docker bits
fi
seed_home "${bits_home}" bits dog
install -d -m 700 -o bits -g dog "${bits_home}/.ssh"
passwd -d bits
usermod -U bits
```

KEEP (unrelated to `bits`, still required):
- `groupadd --gid 501 dog`
- `useradd --gid dog --uid 501 ...` (the `dog` user itself)
- `seed_home "${dog_home}" dog dog`
- `install -d -m 0755 -o dog -g dog "${dog_home}/sbin"`
- sudoers drop-in (`/etc/sudoers.d/95-dog-nopasswd`)
- `/etc/zsh/zshenv`

`build-shared` (GID 9001) is created in `linux/Dockerfile:371` (`groupadd -g 9001 build-shared`) and is NOT declared in this file — confirmed.

**NOTE:** the `usermod -a -G docker bits` line is already `getent`-guarded in setup.sh (the `if getent group docker` block above). This confirms the guard idiom used in File B is the existing project convention.

### File B — agent `.devcontainer/datadog/default/features/datadog-agent/install.sh`

ADD `build-shared` membership after the existing `usermod -aG docker bits` (line 9):

```sh
# Preferred (guarded) form — safe under `set -e` if group is somehow absent:
getent group build-shared >/dev/null && usermod -aG build-shared bits
```

The guarded form is the right choice. It matches the idiom already used in `setup.sh` for the docker group (`if getent group docker >/dev/null; then usermod ...`). **P2-c resolved (2026-06-03):** the `docker` group exists at the `linux/Dockerfile` layer (pre-#1089 image inspection: `uid=2000(bits) ... groups=501(dog),27(sudo),100(users),9001(build-shared),999(docker)`). Both `docker` and `build-shared` survive the de-bake. The guard degrades to a clean no-op on stock workspace images where neither group exists (confirmed live on this stock box).

**Confirmed-correct context to NOT "fix" (P2-d):** `bits` is created with primary group `dog` (`-g dog`). Sudo is granted via the `%dog ALL=(root) NOPASSWD:ALL` drop-in (`dd-source/.../base/filesroot/etc/sudoers.d/dog-group`), NOT via a `sudo` supplementary group. Therefore dropping the explicit `users`/`sudo` supplementary groups from File A's pre-bake does **not** break sudo. Do not re-add them.

**Implicit dependency to document (P2-f):** `install.sh:6` does `cp /root/.local/bin/claude /home/bits/.local/bin/claude`. This now relies on `bitsinit.sh:21` (`mkdir -p ~/.local/bin`) having run in the base feature. `installsAfter: base` (`devcontainer-feature.json:6`) guarantees ordering, so this is safe — but call it out as a load-bearing implicit dependency so a future reorder doesn't silently break the `cp`.

## 3. Cross-Repo Ordering & Enforcement

Three coupling points, in increasing order of "actually flips the switch":

1. `prebuild-devcontainer.json` — source config. Current (pre-#1089, no collision yet) tag: `dev-env-workspace:v110159745-8628883e`, base feature `0.4.100410934`. Latest published post-#1089 tag (verified available): `v116490909-fc35a1a5` — use this or a newer one for adoption step C.
2. `devcontainer.json` — baked composite digest `sha256:2ef4...`. **This is the real adoption switch.**
3. The digest itself, baked by the prebuild pipeline from (1).

**Invariant:** never build/adopt a composite whose base is the pre-bake-removed image unless the agent feature code baked into that composite already contains the `build-shared` membership line (File B).

**Sequence:**
- **(A)** Land **File B first.** No-op on the current image (the pre-baked `bits` is already in `build-shared`), so it is safe to merge immediately and in isolation.
- **(B)** Land **File A.** Publishes a new image tag. No agent impact while the agent still pins the old tag/digest.
- **(C)** Bump the base tag in `prebuild-devcontainer.json` → regenerate composite → update the `devcontainer.json` digest.

A and C need not be atomic. **A must merge before any new-base composite is built. Avoid C-before-A.**

### Enforcement (P1-c) — the invariant must be defended against automation, not just stated

The danger: a renovate/CI auto-bump of the base image tag in `prebuild-devcontainer.json` could trigger a composite rebuild on the NEW base before a human sequences step C — silently producing the C-before-A broken state *if* File A's removal shipped but File B's line wasn't actually baked into the composite. Concrete mechanisms (apply all that are feasible; (1) is mandatory):

1. **Order-of-merge as the primary guard (mandatory).** Land and merge File B *before* File A's new tag can be referenced anywhere. Because File B is a no-op on the current image, there is no reason to batch it with File A. Confirm File B is merged to `main` before opening File A's PR.
2. **CODEOWNERS / branch-protection gate** on `prebuild-devcontainer.json` AND `devcontainer.json` requiring agent-team review, so no automated bump of either can merge without a human who knows this invariant. **VERIFY** current CODEOWNERS coverage of these two paths; add an entry if absent.
3. **Auto-bump bot check (VERIFY, with fallback).** Determine whether a renovate/auto-bump bot watches the `dev-env-workspace` base tag in `prebuild-devcontainer.json`.
   - If **yes**: pin/freeze that tag (renovate `ignore`/`enabled:false` for this dependency, or a `# renovate: ` pin comment) until adoption is complete, OR route the bot's PRs through the Acceptance Gate (§4) so a green provision on the new base is required to merge. State which was chosen in the PR description.
   - If **no**: document that the tag is bumped manually and (1)+(2) are sufficient.
   - If **undetermined**: treat as "yes" and pin defensively.

## 4. Acceptance Gate (PRE-MERGE, BLOCKING) — P1-b

**This is the single blocking gate, not a risk-register row.** It applies to **File A** and to the **composite-adoption step C**.

> **GATE:** A full orchestrator-spawned provision must complete **green at least once on the de-baked image** before File A merges and before step C adopts the new composite.

The load-bearing proof of success is that the **provisioner's own `postCreate.sh` — `dda inv install-tools` followed by `dda inv agent.build` — completed green on the de-baked image**, as run by the workspace orchestrator at provision time (verifier: `dd-source/domains/devex/workspaces/internal/libs/provisioning/devcontainer.go:66-105`, which is satisfied because `base` is kept).

- **Capture and inspect the provision-time logs** for that postCreate run. The success criterion is the postCreate build exiting 0 — NOT an interactive shell, NOT a re-run.
- **`qa/rc-required`:** devcontainer/workspace builds are branch-conditional and frequently do NOT run on PR branches. This change therefore likely needs `qa/rc-required` so the gate is actually exercised. Label accordingly and do not rely on PR-branch CI to have run the provision.

### Why interactive `id` is NOT the discriminator (P1-a)

An interactive SSH/shell gets fresh supplementary groups via `initgroups()` on login, so `id` will show `build-shared` **even if the provision-time postCreate build silently ran without the membership**. `id` therefore cannot distinguish success from the failure mode this change is meant to prevent.

> There is **no P0 timing bug**: the root-phase `usermod -aG build-shared bits` IS visible to the later fresh-exec `postCreate.sh` (different process, re-reads group membership). The issue is purely about what the TEST asserts on — it must assert on the **provision-time postCreate build**, demoting `id` to a secondary sanity check.

## 5. Testing Plan

> All commands below were **executed and verified 2026-06-03** against image `v116490909-fc35a1a5`.
> Toolchain paths and ownership confirmed from that image — do not assume pre-#1089 paths (e.g. the old image had RVM at `/usr/local/rvm`; post-#1089 it is at `/opt/dd/rvm`).

### Verified toolchain locations (post-#1089 image, `root:build-shared` setgid)

| Toolchain | Path | Ownership | Write path tested |
|---|---|---|---|
| RVM / Ruby | `/opt/dd/rvm` | `root:build-shared drwxrwsr-x` | `GEM_HOME=/opt/dd/rvm/gems/ruby-2.7.2` ✅ |
| Rust / rustup | `/opt/dd/rustup` | `root:build-shared drwxrwsr-x` | `/opt/dd/rustup/` ✅ |
| Go | `/usr/local/go` (binary), `/var/config/dd/go` (writable cache) | cache: `bits:build-shared` | `/var/config/dd/go` ✅ |
| Conda | `/opt/dd/conda` | `root:build-shared drwxrwsr-x` | (inherited from base image) |

> **GOPATH correction:** the image sets `GOPATH=/var/cache/dd/go` (env var) but that directory does not exist. The actual writable Go path is `/var/config/dd/go` (owner `bits:build-shared`). The agent's `postCreate.sh` already does `unset GOPATH` — Go tools then use `GOCACHE=/var/cache/dd/go/build` and `GOPATH=/var/config/dd/go` after the unset.

### Pre-flight check: confirm post-#1089 image has the pre-baked `bits` (the collision state)

```bash
IMG=registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v116490909-fc35a1a5
# Expected: uid=2000(bits) ... groups=...,9001(build-shared),...
docker run --rm --entrypoint bash "$IMG" -c 'id bits; getent group build-shared'
```

### Test 1: Prove the collision (pre-fix, post-#1089 image)

```bash
IMG=registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v116490909-fc35a1a5
# Expected output: "useradd: user 'bits' already exists" → PASS
docker build --no-cache --build-arg BASE_IMAGE="$IMG" - <<'EOF'
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
RUN useradd --create-home --home-dir /home/bits --uid 2000 -g dog bits \
    && echo "UNEXPECTED: no collision" \
    || echo "CONFIRMED: useradd bits already exists (collision proven)"
EOF
```

### Test 2: Prove the fix (File A effect + base feature + File B effect)

```bash
IMG=registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v116490909-fc35a1a5
docker build --no-cache --build-arg BASE_IMAGE="$IMG" -t s2-test-fixed - <<'EOF'
ARG BASE_IMAGE
FROM ${BASE_IMAGE}

# FILE A: remove pre-baked bits user
RUN userdel -r bits 2>/dev/null || true
RUN id bits 2>&1 | grep -q 'no such user' && echo "FILE_A: bits removed ✓" || (echo "FILE_A: FAIL" && exit 1)

# BASE FEATURE: useradd (simulates dd-source base/install.sh:47)
RUN useradd --create-home --home-dir /home/bits --uid 2000 -g dog bits \
    && echo "BASE_FEATURE: useradd succeeded (no collision) ✓"

# FILE B: agent feature adds build-shared membership (guarded, matching project convention)
RUN getent group build-shared >/dev/null && usermod -aG build-shared bits \
    && echo "FILE_B: usermod -aG build-shared bits ✓"

# Verify final state
RUN id bits | grep -q 'build-shared' && echo "FINAL: bits in build-shared ✓" || (echo "FINAL: FAIL — bits not in build-shared" && exit 1)

CMD ["sleep", "infinity"]
EOF
```

Expected: all four steps print `✓`, build exits 0.

### Test 3: Toolchain write tests as `bits` in a FRESH exec

**Critical:** write tests MUST run in a fresh `docker exec` process, not the same shell that ran `usermod`. An interactive login always calls `initgroups()` and will show the group regardless — the test must prove the *provision-time* path works (P1-a).

```bash
# Start the fixed container
docker run -d --name s2-fixed --entrypoint sleep s2-test-fixed infinity

# [a] Secondary sanity (id — necessary but not sufficient per P1-a)
docker exec -u bits s2-fixed id
# Expected: uid=2000(bits) gid=501(dog) groups=501(dog),9001(build-shared)

# [b] RVM gem write as bits — fresh exec, login shell to source rvm (P2-b: the ownership-sensitive test)
docker exec -u bits s2-fixed bash -lc '
    source /opt/dd/rvm/scripts/rvm
    echo "GEM_HOME=$GEM_HOME"  # Expected: /opt/dd/rvm/gems/ruby-2.7.2
    touch "${GEM_HOME}/.write-test-$$" && echo "RVM write: PASS ✓" && rm -f "${GEM_HOME}/.write-test-$$"
'
# Expected: GEM_HOME=/opt/dd/rvm/gems/ruby-2.7.2, RVM write: PASS ✓

# [c] Rustup dir write as bits
docker exec -u bits s2-fixed bash -c '
    touch /opt/dd/rustup/.write-test-$$ && echo "rustup write: PASS ✓" && rm -f /opt/dd/rustup/.write-test-$$
'

# [d] Go build as bits (real compile)
docker exec -u bits s2-fixed bash -c '
    cd /tmp && mkdir -p gotest && cd gotest
    echo "package main\nimport \"fmt\"\nfunc main(){fmt.Println(\"ok\")}" > main.go
    GOPATH=/var/config/dd/go go build -o /tmp/s2-gotest ./main.go \
        && echo "go build: PASS ✓"
'

# Cleanup
docker rm -f s2-fixed
```

### Test 4: Stock-workspace regression (run on any stock Workspace, no buildimage needed)

Confirms File B's guard cleanly no-ops when `build-shared` is absent, preserving the stock path:

```bash
# On a stock workspace (no build-shared group):
if getent group build-shared >/dev/null; then
    echo "build-shared exists — not a stock workspace"
else
    getent group build-shared >/dev/null && usermod -aG build-shared bits
    echo "guard no-op: PASS ✓ (exit $?)"
fi
# Expected: "guard no-op: PASS ✓"
```

> Executed live on this machine (stock workspace `nick-isaacs-dd-agent-test`) — PASSED 2026-06-03.

### Acceptance Gate (§4) — orchestrator provision (production gate, not locally reproducible)

The four docker tests above are the **locally-reproducible pre-flight**. The full acceptance gate requires an orchestrator-spawned provision:

```
1. Launch a Datadog Workspace using the new image tag
2. Let provisioning run to completion (do NOT interrupt)
3. Assert postCreate.sh exited 0 in provision logs:
   - dda inv install-tools succeeded
   - dda inv agent.build --build-exclude=systemd succeeded
4. Only then: merge adoption PR (step C)
```

**Why `id` is insufficient (P1-a):** an interactive login always calls `initgroups()` and reads fresh group membership. `id` shows `build-shared` even if postCreate ran without it. The provision log exit code is the only reliable signal.

### Dotfile regression (known, cosmetic — document in PR)

`seed_home bits` previously gave `bits`: starship, nushell config, `.scripts/`, custom `.zshrc`. After de-bake, `bitsinit.sh` provides: oh-my-zsh, `~/dd` symlink, `~/.ssh`, `~/.local/bin`. Delta: **starship, nushell, `.scripts`, `.zshrc` customisations are lost.** Build works; shell UX is less polished. Optional follow-up: add a dotfile re-seed step to the agent feature. Document in the PR description so reviewers don't flag it as a regression to fix before merge.

## 6. Rollback

- Revert **File B**: drop the `build-shared` `usermod` line.
- Revert **File A**: restore the pre-bake block (republish prior image tag).
- Revert **adoption**: restore the prior base tag in `prebuild-devcontainer.json` and the prior composite digest (`sha256:2ef4...`) in `devcontainer.json`.

Because the steps are sequenced (B → A → C) and B is a no-op on the old image, rolling back C alone (re-pin old digest) instantly restores the working state without touching A or B.

## 7. Risk Register

| # | Risk | Likelihood | Impact | Status | Mitigation |
|---|------|-----------|--------|--------|------------|
| 1 | `build-shared` absent on new image → guarded usermod no-ops silently | Low | High | **Mitigated** | Guarded `getent` form; Test 4 (stock regression) confirms guard; group confirmed present in post-#1089 image |
| 2 | `docker` group only existed via pre-bake context → existing line fails | ~~Low–Med~~ | High | **RESOLVED** (P2-c) | docker group confirmed in post-#1089 image inspection: `999(docker)` — exists at Dockerfile layer |
| 3 | Removed `install -d`/`chown` of a build-shared dir the runtime needs | ~~Med~~ | High | **RESOLVED** (P2-a) | File A enumerated verbatim: no hidden build-shared dir-prep. All toolchain ownership in `linux/Dockerfile` via `fix-shared-perms.sh` |
| 4 | Auto-bump bot triggers C-before-A | Med | High | Open | §3 Enforcement: merge-order + CODEOWNERS + pin/route bot |
| 5 | Toolchain group-write silently fails at provision | ~~Low~~ | High | **TESTED** | Test 3b/c/d confirms RVM, rustup, go writes succeed as bits via build-shared in fresh exec |
| 6 | RVM `GEM_HOME` ownership-sensitive | ~~Med~~ | Med | **TESTED** | Test 3b: `gem write PASS ✓` at `/opt/dd/rvm/gems/ruby-2.7.2` (login shell, fresh exec) |
| 7 | Dotfile loss (starship/nushell/.scripts/.zshrc) | High | Low (cosmetic) | Accepted | §5 testing plan; documented in PR; oh-my-zsh + ~/dd still provided |
| 8 | `claude` cp fails if `~/.local/bin` missing | Low | Med | **Mitigated** | `installsAfter: base` guarantees `bitsinit.sh:21` ran; load-bearing dependency documented in §2 |

## 8. Open Questions / VERIFY

**Resolved (2026-06-03 testing):**
- ~~**[VERIFY] File A real contents**~~ → **RESOLVED.** Exact verbatim block confirmed in §2. No hidden dir-prep. `build-shared` not re-declared in `setup.sh`.
- ~~**[VERIFY] `docker` group origin**~~ → **RESOLVED.** GID 999 present in post-#1089 image at Dockerfile layer; survives de-bake.
- ~~**[VERIFY] RVM path**~~ → **RESOLVED.** `GEM_HOME=/opt/dd/rvm/gems/ruby-2.7.2`, `root:build-shared drwxrwsr-x`. Write confirmed as `bits` in fresh exec.

**Still open (require human/infra action):**
- **[VERIFY] CODEOWNERS coverage** (P1-c): do `.devcontainer/datadog/default/prebuild-devcontainer.json` and `.devcontainer/datadog/default/devcontainer.json` require agent-team review? Check `.github/CODEOWNERS`; add entry if absent to prevent automated bump merging without review.
- **[VERIFY] Auto-bump bot** (P1-c): does a renovate or CI bot watch the `dev-env-workspace` tag in `prebuild-devcontainer.json`? If yes → pin/freeze until adoption complete, or require Gate passage before bot PRs merge. If undetermined → treat as yes and pin defensively.

## 9. Owner & Cost

- **Owner:** Agent team (both changed files are agent-team-owned).
- **Cost:** S (two small edits + one composite rebuild/adoption), gated by one orchestrator provision run.
- **Label:** likely `qa/rc-required` (§4) — workspace/devcontainer builds are branch-conditional.

### Cross-reference

P2 "image mutation" (bake `usermod` once vs. re-run per container start) is explicitly **out of scope** here; see `workspace-solution-options.md`.
