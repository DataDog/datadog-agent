# Workspace Integration — Solution Options Comparison

> Companion to `workspace-integration.md` (which details the base-feature-split path, = S4 here).
> Produced by a research fan-out: one decision-context agent + three solution agents (S1/S2/S4),
> all evidence-driven against dd-source, datadog-agent-buildimages, and the Go provisioner.
> Framing pressure-tested by advisor: sufficiency claims are deferred to stakeholders, not asserted.

## The two problems (they are not the same problem)

The decision-context research established that the agent team's motivation for using the
`dev-env-workspace` buildimage as their Workspace base is **bit-for-bit CI parity** (Kevin
Fairise + Ofek Lev, verbatim). That splits the ask into two distinct problems:

- **P1 — the hard collision.** Buildimages (post-PR #1089) pre-bakes the `bits` user (UID 2000);
  the base feature's `install.sh:47` runs `useradd ... bits` → fails → provisioning dies.
  This is a *correctness* failure that blocks everything.
- **P2 — image mutation.** Ofek: *"we already manage the versions of software we use, the
  permissions of the logged-in SSH user, etc. such that install.sh and other scripts corrupt
  our image."* The base feature installs ~15 overlapping **utility** tool versions and mutates
  the image. This is a *stewardship* concern about the platform writing into a managed image.

**A solution can fix P1 without touching P2.** This is the key discriminator below.

### Parity fact (verified, primary source)

The base `install.sh` + `bitsinit.sh` install **no parity-critical build toolchain** — no Go,
Rust, Ruby/RVM, Conda-Python, glibc, or C cross-toolchain. They install OS utilities
(zsh, fish, vim, git, cmake, jq…), ops tools (kubectl 1.31.1, vault 1.17.5, aws-vault, awscli,
crane, bazelisk 1.12.0, buildifier 8.2.0, uv 0.9.16), Datadog internal dev tools (via
`update-tool`: atlas, bldg, bzl, dd-gopls, ddtool, dogbrew, fabric, git-dd…), and `gem install
bundle` / `pip install pre-commit` against system ruby/python. `chown -R bits:dog` touches only
`${bitsHome}` and `/opt/dogbrew` — never toolchain trees.

→ **The base feature running in full does not break the agent build's bit-for-bit parity.**
   It *does* layer overlapping utility versions onto the image (that is P2, not a parity break).
   Residual unknown: `update-tool`'s transitive install closure is a binary we could not fully
   inspect; the named tool set contains no language toolchain.

---

## The three solutions

### S2 — Drop the `bits` user pre-bake; own toolchains by `build-shared` group (NOT user)

**Idea.** PR #1089 pre-bakes `bits` partly for convenience; the toolchains are *already* owned
`root:build-shared` + setgid (proven by `linux/scripts/fix-shared-perms.sh` — zero `chown bits`
on any toolchain; proven-by-existence by the `dd`/`dd-build` users that access everything,
RVM included, via group membership alone). So buildimages can **stop creating the `bits` user**;
the base feature creates it at runtime as it always has → **no collision**.

**The one caveat (load-bearing).** `bits`'s toolchain *write* access comes from `build-shared`
group **membership**, which today is granted only by the pre-bake line. The base feature creates
`bits` with primary group `dog` and never adds it to `build-shared`. The shared roots are
`g+rwxs,o+rx` — world has read+execute but **not write**, and every build *creates* files under
them (`GOPATH=/var/cache/dd/go`, `CARGO_HOME`, `GEM_HOME`, omnibus caches). So membership is
mandatory, not optional. Fix: one line in the **agent-owned** feature install.sh, which already
does the identical pattern (`usermod -aG docker bits`):

```sh
usermod -aG build-shared bits
```

**Changes (all agent-owned — zero platform/dd-source/provisioner changes):**
1. `datadog-agent-buildimages` `dev-envs/linux/variants/workspaces/setup.sh`: remove the `bits`
   `useradd` + `usermod -aG docker bits` + `seed_home bits` + `.ssh` pre-bake block.
2. `datadog-agent` `.devcontainer/datadog/default/features/datadog-agent/install.sh`: add
   `usermod -aG build-shared bits` (+ optional dotfile re-seed from `/home/dd` if the rich
   shell UX is wanted — `useradd --create-home` only pulls empty `/etc/skel`).

**No recursive chown** at provision (group membership is O(1); toolchains stay baked, untouched).
**Build verifier** (`devcontainer.go:66-105`) is satisfied for free — S2 keeps the `base` feature,
which is branch (b) of the verifier, and port 22 stays forwarded.

- **Fixes P1:** yes, at the source. **Fixes P2:** **no** — base feature still installs its tools.
- **Owner:** agent team only. **Cost:** S. **Confidence:** HIGH on the ownership kill-switch and
  the membership gap (primary source); MED pending a smoke build.
- **Verify before commit:** build `dev-env-workspace` with the pre-bake removed + `usermod` added;
  run a `gem install` / cargo build / `dda inv` as `bits`; confirm writes succeed, no chown storm.

### S4 — Split `base`; introduce a minimal provisioning-only contract (= `workspace-integration.md`)

**Idea.** Kevin's stated preference: `base` layer 1 = create user + install dev tools (skippable
by images that pre-bake); layer 2 = minimal contract that assumes the user exists and only
installs the provisioner scaffolding. Ofek has an ABI-only POC (`dd-source` branch
`ofek/workspaces`, feature `workspace-abi`, no PR yet).

**This is the only path that addresses P2** — it stops the base feature from mutating the
managed image at all.

**Corrections this research forces into `workspace-integration.md` (it is currently incomplete):**
1. The minimal contract must ship **THREE** provisioner scripts, not one:
   - `/opt/doghome/sbin/adduser.sh` (`user.go:12`)
   - `/opt/doghome/sbin/setup_ide_backends.sh` (`ide_backend_setup.go:12`) ← what actually broke #50576
   - `/opt/doghome/sbin/install_dotfiles.sh` (`dotfiles.go:22`)
   plus transitively `/usr/local/lib/git-config-tool` (invoked by `adduser.sh`).
2. It must satisfy the **build verifier** (`devcontainer.go:66-105`): either identify as the
   `base` feature (ID + Name `Datadog Workspaces Components`, or set
   `dog.infra.workspace.base-feature-version`) **or** the image must carry label
   `dog.infra.workspace.base-image` + `overrideCommand:false`; and port 22 must be forwarded.
   Ofek's POC sets `workspace-abi-feature-version`, which satisfies **neither** branch → would
   fail the verifier as-is, and it ships none of the three scripts → reproduces the #50576 failure.
3. UID 2000 / GID 501 are hard-coded **again** in `dotfiles.go:37-38` (setpriv), beyond `user.go`.
4. Bazel sharing: filegroup-in-`srcs` is **confirmed** to work (`devcontainer_feature.bzl:70-72`
   passes `srcs` to `pkg_tar`, which accepts cross-package labels and strips by package-relative
   path). `glob(["**/*"])` is what made the POC duplicate files. Do not use symlinks.

- **Fixes P1:** yes. **Fixes P2:** **yes** (the only one that does).
- **Owner:** platform + agent. **Cost:** M. **Confidence:** HIGH on contract + gaps; MED on Bazel
  path-stripping and the `dda`/`invoke` regression mechanism.
- **Interim sub-option:** add a `createUser: false` devcontainer-feature **option** to `base`
  (it currently exposes no `options` block) — an explicit contract, cleaner than the blind
  `id -u` guard in `workspace-integration.md` Phase 1. Still doesn't fix P2.

### S1 — Declarative, version-pinned dep install (laptop + workspace + CI share one source of truth)

**Idea.** Make toolchain installation a pinned declarative spec (e.g. `mise` `.tool-versions`)
that is the shared source of truth the CI image is *built from* — so parity holds by construction
and the same path runs on a laptop. **Orthogonal to P1/P2** (it doesn't touch the collision).

**Verdict: PARTIALLY VIABLE — subset only.** Full replacement of the buildimage is BLOCKED at the
native/release layer: glibc + crosstool-ng cross-toolchains, Conda-embedded CPython, omnibus RVM
Ruby — Linux-only, root-required, built-from-source. The inversion that kills the full version:
the deps that are *easy* to declare cross-platform (Go/Rust/Python/Ruby runtimes) are exactly the
ones where strict parity barely matters; the parity-critical deps can't be declared cross-platform
and are a *shared binary artifact*, not a recipe. Today there are already **two** version sources
reconciled manually (`.go-version` etc. ↔ buildimages `docker-bake.hcl`, synced by
`dda inv update-go`).

**Defensible subset (worth doing on its own merits):** one pinned manager as shared SoT for
Go/Rust/Python/Ruby across laptop + workspace + CI; keep the baked image for the release/native
layer. Workspace **prebuild caching** absorbs the install cost on Linux (confirmed mechanism).
Laptop benefit is real but bounded — current laptop setup (`docs/public/setup/manual.md`) is
manual and self-deprecating, so unifying the managed-runtime subset genuinely helps; laptops can
never get the native/release path.

- **Fixes P1:** no. **Fixes P2:** no. **Independent value:** laptop+workspace toolchain unification.
- **Owner:** both. **Cost:** S–M (subset) / L (full, and the full version is likely infeasible).
  **Confidence:** HIGH on the structural verdict.

---

## Comparison

| | S2 group-ownership | S4 split feature | S1 declarative subset |
|---|---|---|---|
| Fixes P1 (collision) | ✅ at source | ✅ | ❌ |
| Fixes P2 (image mutation) | ❌ | ✅ (only one) | ❌ |
| Touches dd-source / provisioner | **No** | Yes | No |
| Platform coordination | **None** | Required (Kevin's team) | Some (prebuild + bake SoT) |
| Preserves bit-for-bit parity | ✅ | ✅ | ✅ (subset) / ❌ (full) |
| Independent benefit | — | — | laptop+workspace unification |
| Cost | **S** | M | S–M (subset) |
| Owner | agent team | platform + agent | both |
| Confidence | HIGH / MED (smoke) | HIGH / MED (bazel) | HIGH |

## Recommendation (decision deferred where it belongs)

1. **Ship S2 now to unblock.** It deletes P1 at the source for cost S, entirely within
   agent-owned files, with no platform changes and no fork — and parity is preserved.
   This is strictly cheaper and lower-risk than `workspace-integration.md` Phase 1
   (which edits the shared base feature in dd-source).
2. **Whether S2 is *sufficient* is the agent team's call, not ours to assert.** S2 leaves P2
   fully unaddressed: the base feature still layers its managed utility versions onto the image
   and mutates it — the exact thing Ofek flagged. The question to put to the agent team +
   platform: *do you accept the base feature's tool installs and image mutations, or do you
   require the image to stay untouched?* If accepted → S2 is the end state. If not → S4.
3. **S4 is the answer iff P2 must be solved**, and it is Kevin's stated direction. If pursued,
   `workspace-integration.md` must first be corrected for the three-script contract and the
   build verifier (above) — as written it would reproduce the #50576 failure.
4. **S1 (subset) is worth doing regardless** of which P1/P2 path wins — it's the only one that
   improves the laptop story, and it can proceed in parallel on its own track.

## Ranking (recommended order of action; value-to-cost-to-risk)

> Criteria: fixes P1 (collision/blocker) · fixes P2 (image mutation) · cost + blast radius ·
> platform coordination/fork burden · stakeholder alignment · independent value.
> S2 and S4 are sequential-compatible (S2 now → S4 later if P2 is confirmed), not either/or.
> S1 is an orthogonal track. PR #50576 is included as the rejected baseline.

**1. S2 — group-ownership, drop the `bits` pre-bake.**
Fixes the actual blocker (P1) *at its source*, cost S, entirely in agent-owned files, zero
dd-source / provisioner / platform changes, no fork to maintain, parity preserved, build verifier
satisfied for free (it keeps `base`). Nothing else matches that cost/risk profile. It ranks #1
even though it doesn't fix P2, because P2 is a stewardship preference while P1 is a hard failure —
and S2 buys the team a working workspace *today* without committing anyone to platform work.
Only debt: one smoke build to confirm group-write, and the dotfile re-seed decision.

**2. S4 — split `base` into a minimal contract (`base-compat` / `workspace-abi`).**
The *only* option that fixes P2, and it is Kevin's explicitly stated preferred direction — so it
has the strongest long-term stakeholder alignment and is the proper end state if the managed image
must stay untouched. Ranked below S2 only because it costs more (M), is platform-owned, carries a
fork/contract-maintenance burden, and S2 may make it unnecessary. As the *strategic* target it is
arguably #1; as the *next thing to do* it is #2. Must first absorb the three-script + build-verifier
corrections or it reproduces the #50576 failure.

**3. `createUser: false` option on `base` (the principled S3 interim).**
Fixes P1 with an explicit, self-documenting contract — strictly better than the blind guard below.
Ranked #3 not #1 because it still requires a dd-source change to the shared base feature (add an
`options` block + gate `useradd`), whereas S2 needs no platform change at all. It's the best
*fallback* if S2's group-ownership smoke test fails or buildimages can't drop the pre-bake. Does
not fix P2.

**4. Phase-1 blind `id -u` guard (current `workspace-integration.md` Phase 1).**
Cheapest dd-source change (two lines) and unblocks P1. Ranked below `createUser:false` because it
*silently masks* UID/group drift — if the buildimage's `bits` ever diverges from the UID 2000 /
GID 501 the feature and provisioner hard-code, the guard hides it and the failure surfaces later,
subtler. Acceptable as an emergency unblock, inferior as a durable contract. Does not fix P2.

**5. S1 subset — declarative pinned Go/Rust/Python/Ruby as shared source of truth.**
Ranked here, mid-pack, with a caveat: it fixes *neither* P1 nor P2, so it cannot be the
workspace-unblock answer and is not competing with #1–#4. But it is the *only* explored option with
independent value — it improves the laptop story (today manual + self-deprecating) and unifies
laptop/workspace/CI versions for the runtimes where that's achievable. It should proceed on its own
track regardless of which P1/P2 path wins. Ranked above #6/#7 because it is genuinely worth doing;
below #1–#4 because it doesn't touch the stated blocker.

**6. S1 full — replace the buildimage with a declarative spec.**
Research verdict: BLOCKED. The native/release layer (glibc, crosstool-ng cross-toolchains,
Conda-embedded CPython, omnibus RVM Ruby) is Linux-only, root-required, built-from-source, and is a
*shared binary artifact* a cross-platform recipe cannot reproduce — attempting it would break the
very parity that motivates the whole effort. Ranked second-to-last: not viable as stated, kept only
to record why "just make it all declarative" is a dead end.

**7. PR #50576 — remove the base feature entirely (rejected baseline).**
Last. Already rejected by Kevin ("keeping compatibility would be a nightmare") and empirically
broken: missing `setup_ide_backends.sh`, `dda` missing `invoke`, container crash loop. It is the
anti-pattern the entire provisioner contract exists to prevent. Listed only to anchor the bottom.

## Open questions for the stakeholder meeting (Kevin / Ofek / Andrew)

- Does the agent team accept the base feature's managed-tool installs + image mutation (→ S2
  sufficient), or must the managed image stay untouched (→ S4)?
- Who owns keeping the buildimage's baked identity in sync if the platform bumps the `bits` UID
  again (it already went 1000 → 2000, per WRK-1850)? (Affects S2 and S4 equally.)
- Are the rich shell dotfiles (oh-my-zsh, starship) wanted for `bits` in Workspaces? (Drives
  whether S2 needs the dotfile re-seed step.)
- `update-tool` transitive install closure — confirm it pulls no language toolchain (residual
  unknown; doesn't change P1/P2 but firms the parity claim).
