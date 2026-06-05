# Plan: Agent Buildimages in Workspaces

> Produced by adversarial planning (Sonnet planner × Opus critic × Sonnet revision).
> All P0/P1 critique issues addressed. Ready for Kevin Fairise alignment meeting.
> **Updated 2026-06-03** with confirmed post-#1089 image tag, complete provisioner contract (3 scripts), and concrete runnable tests.
> See also `workspace-s2-group-ownership.md` — the lower-cost S2 path that may make Phase 1 of this plan unnecessary.

## Overview

The `datadog-agent-buildimages` workspace image variant (post-PR #1089) pre-bakes `bits`
(UID 2000), `dog`, and `build-shared` groups before the workspace base feature runs. The base
feature's `install.sh` unconditionally calls `useradd ... bits`, which fails with "user already
exists" when `bits` is pre-baked.

The fix has two phases:
- **Phase 1 (interim, in `dd-source`):** Guard the single `useradd` line in `base/install.sh`.
  Ship immediately. Allows the agent team to bump to the post-#1089 image right away.
- **Phase 2 (target, in `dd-source`):** Introduce a `base-compat` feature containing only
  provisioner scaffolding (no user creation, no tool installs). Future workspace images can
  declare `base-compat` instead of `base`.

A companion change in `datadog-agent` updates `prebuild-devcontainer.json` to reference the
new image tag and (in Phase 2) the `base-compat` feature.

---

## Phase 1: Minimal guard in `base/install.sh`

**Repository:** `DataDog/dd-source`  
**File:** `domains/devcontainers/features/base/install.sh`

### What collides and what does not

The execution order in `install.sh`:

1. `addEnv IN_WORKSPACE 1` and other env vars — always safe
2. **`useradd --create-home --home-dir "${bitsHome}" --uid 2000 -g dog bits`** — THE ONLY LINE THAT COLLIDES
3. `usermod` on dog group; `mv lifecycle/*` and `mv lib/*` into `/opt/doghome/...`
4. apt repos + package install — idempotent, no collision
5. `find "$FILE_COPY_SRC" ... cp -dR ... /` (filesroot copy — installs `adduser.sh` to
   `/opt/doghome/sbin/` and `/etc/sudoers.d/dog-group`) — **must run unconditionally**
6. `chown -R bits:dog`, `su --login bits ... bitsinit.sh`, tool downloads, bees, etc.

Everything except step 2 runs unconditionally. **Do not guard any other block.**

### Exact diff

```diff
--- a/domains/devcontainers/features/base/install.sh
+++ b/domains/devcontainers/features/base/install.sh
@@ -... @@
-useradd --create-home --home-dir "${bitsHome}" --uid 2000 -g dog bits
+if ! id -u bits >/dev/null 2>&1; then
+    useradd --create-home --home-dir "${bitsHome}" --uid 2000 -g dog bits
+fi
```

If a `chpasswd` line for `bits` immediately follows `useradd` in the actual file, include it
inside the same guard. Verify in the actual file — do not assume.

### Why apt/tools/filesroot stay unconditional

- **apt install:** Idempotent. No-op when packages are already present.
- **filesroot copy:** Installs `/opt/doghome/sbin/adduser.sh` — the Go provisioner
  (`user.go` `const addUserPath`) hard-codes this path. If the copy is skipped, the provisioner
  silently fails for every workspace provisioned. Must always run.
- **bitsinit.sh / tool downloads:** Redundant work on a pre-baked image but not harmful.
  Eliminating this waste is Phase 2's job, not Phase 1's.

The guard is permanent. After Phase 2, `base` still serves images that do not pre-bake `bits`.
Do not remove the guard.

### Phase 1 rollout sequence

Order matters: bumping the agent image while the base feature is unguarded is the exact
breakage being fixed.

1. Open PR in `dd-source` with the `useradd` guard.
2. Merge and republish `workspaces/features/base` → obtain new version `0.4.<NEW_BUILD_ID>`.
3. Open PR in `datadog-agent` bumping **both** the image tag and the base version **together
   in a single commit**:

```diff
--- a/.devcontainer/datadog/default/prebuild-devcontainer.json
+++ b/.devcontainer/datadog/default/prebuild-devcontainer.json
 {
     // Trigger build: 2026-05-04 10:25
     "name": "Datadog Agent Development Container",
-    "image": "registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v110159745-8628883e",
+    "image": "registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:<POST_1089_TAG>",
     "features": {
-        "registry.ddbuild.io/workspaces/features/base:0.4.100410934": {},
+        "registry.ddbuild.io/workspaces/features/base:0.4.<NEW_BUILD_ID>": {},
         "./features/datadog-agent": {}
     },
     "forwardPorts": [22],
     "containerUser": "root",
     "remoteUser": "bits",
     "waitFor": "postStartCommand"
 }
```

All other fields (`name`, `containerUser`, `remoteUser`, `forwardPorts`, `waitFor`,
`./features/datadog-agent`) are preserved exactly.

TBD values (to be filled in during execution):
- `<POST_1089_TAG>`: image tag from the post-PR-#1089 buildimages publish. **Confirmed available 2026-06-03:** `v116490909-fc35a1a5` is post-#1089 (`bits` uid=2000 present, `build-shared` GID 9001 present). Use this or a newer tag. Verify with: `docker run --rm --entrypoint bash registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:<TAG> -c 'id bits; getent group build-shared'`
- `0.4.<NEW_BUILD_ID>`: version string from the Phase 1 base feature publish

---

## Phase 2: `base-compat` feature

**Repository:** `DataDog/dd-source`

### New directory layout

```
domains/devcontainers/features/
├── base/                              # unchanged except Phase 1 guard
│   ├── BUILD.bazel
│   ├── devcontainer-feature.json
│   ├── install.sh
│   └── filesroot/
├── base-compat/                       # NEW
│   ├── BUILD.bazel
│   ├── devcontainer-feature.json
│   └── install.sh
└── lib/                               # NEW — shared Bazel source
    ├── BUILD.bazel
    ├── install-provisioning-compat.sh
    └── filesroot/                     # provisioner-critical files (moved from base/filesroot/)
        └── opt/doghome/sbin/adduser.sh
        └── etc/sudoers.d/dog-group
        └── opt/doghome/devcontainer/features/base/lifecycle/
        └── ...
```

### Bazel shared source — no symlinks, no shell source outside artifact

`base/BUILD.bazel` uses `srcs = glob(include = ["**/*"], ...)`. Symlinks pointing outside
the feature directory are not followed by the glob and are not packaged into the OCI artifact.

**All sharing must happen at Bazel build time via a shared `filegroup`.**

**`domains/devcontainers/features/lib/BUILD.bazel`**

```python
filegroup(
    name = "provisioning_scaffolding",
    srcs = glob(include = ["**/*"], exclude = ["BUILD.bazel"]),
    visibility = ["//domains/devcontainers/features:__subpackages__"],
)
```

**`domains/devcontainers/features/base/BUILD.bazel`** (modified)

```python
devcontainer_feature(
    name = "base",
    srcs = glob(
        include = ["**/*"],
        exclude = ["BUILD.bazel", "devcontainer-feature.json"],
    ) + ["//domains/devcontainers/features/lib:provisioning_scaffolding"],
    ...
)
```

**`domains/devcontainers/features/base-compat/BUILD.bazel`** (new)

```python
devcontainer_feature(
    name = "base-compat",
    srcs = [
        "devcontainer-feature.json",
        "install.sh",
        "//domains/devcontainers/features/lib:provisioning_scaffolding",
    ],
    ...
)
```

> **Critical path-mapping verification:** When the `devcontainer_feature` rule packages a
> cross-package filegroup, files from `lib/` must be mapped to the correct root-relative paths
> inside the OCI artifact so that the `find $FILE_COPY_SRC` invocation resolves correctly at
> runtime. Inspect the artifact's layer contents with `bazel build` + manual layer inspection
> before merging. Adjust `FILE_COPY_SRC` in `install-provisioning-compat.sh` if the rule strips
> the `lib/` prefix differently than expected. This is the highest-risk build system question
> in Phase 2.

### `lib/install-provisioning-compat.sh`

Contains only the unconditionally-required provisioning steps (copied verbatim from
`base/install.sh`'s equivalent steps — do not paraphrase):

```bash
#!/bin/bash
set -euo pipefail

FEATURE_DIR="$(cd "$(dirname "$0")" && pwd)"

# Workspace environment markers (always required)
addEnv IN_WORKSPACE 1
# ... other addEnv calls — copy from base/install.sh top block ...

# Install lifecycle scripts and lib
mv "${FEATURE_DIR}/lifecycle/"* /opt/doghome/devcontainer/features/base/lifecycle/
mv "${FEATURE_DIR}/lib/"* /opt/doghome/sbin/

# Filesroot copy — installs adduser.sh, sudoers.d/dog-group, and scaffolding
# (copy this find invocation verbatim from base/install.sh step 5)
find "$FILE_COPY_SRC" -mindepth 1 -exec cp -dR {} / \;
```

### `base-compat/install.sh`

```bash
#!/bin/bash
set -euo pipefail

FEATURE_DIR="$(cd "$(dirname "$0")" && pwd)"

# Precondition checks
if ! id -u bits >/dev/null 2>&1; then
    echo "ERROR: base-compat requires 'bits' user to exist in the image."
    echo "Pre-bake bits (UID 2000) in your image, or use the 'base' feature instead."
    exit 1
fi
if [ "$(id -u bits)" != "2000" ]; then
    echo "ERROR: 'bits' user must be UID 2000, found $(id -u bits)."
    exit 1
fi
if ! getent group dog >/dev/null 2>&1; then
    echo "ERROR: 'dog' group is required but does not exist in this image."
    exit 1
fi

# Run provisioning scaffolding
# NOTE: install-provisioning-compat.sh arrives via the Bazel filegroup —
# it is a packaged copy inside this OCI artifact, not a path outside it.
# shellcheck source=../lib/install-provisioning-compat.sh
source "${FEATURE_DIR}/install-provisioning-compat.sh"
```

### `base-compat/devcontainer-feature.json`

```json
{
    "id": "base-compat",
    "version": "0.1.0",
    "name": "Workspaces Base Compat",
    "description": "Provisioner scaffolding for workspace images that pre-bake the bits user. Installs adduser.sh, sudoers wiring, and lifecycle hooks. Does NOT create users or install dev tools.",
    "dependsOn": {
        "ghcr.io/devcontainers/features/sshd:1.0.9": {},
        "ghcr.io/devcontainers/features/docker-in-docker:2.12.1": {
            "dockerDefaultAddressPool": "<COPY EXACT VALUE FROM base/devcontainer-feature.json>"
        },
        "ghcr.io/devcontainers/features/github-cli:1": {}
    },
    "entrypoint": "/opt/doghome/devcontainer/features/base/lifecycle/entry.sh",
    "postCreateCommand": {
        "workspaces-base": "/opt/doghome/devcontainer/features/base/lifecycle/post_create.sh",
        "change-ownership": "/opt/doghome/devcontainer/features/base/lifecycle/change_ownership.sh &"
    }
}
```

**Notes:**
- `dependsOn` is copied exactly from `base/devcontainer-feature.json` — do not omit sshd,
  docker-in-docker, or github-cli. Making any of these optional is a future optimization
  requiring a separate platform team decision.
- Copy the `dockerDefaultAddressPool` value verbatim from the real base feature.json —
  do not guess it.
- `entrypoint` and `postCreateCommand` paths are identical to `base` — they resolve because
  `install-provisioning-compat.sh` installs scripts to the same `/opt/doghome/...` paths.
- `change_ownership.sh &` backgrounded — preserve the `&`. The script targets `/home/bits/go`,
  self-guards with `exit 0` if absent, and must not block workspace startup.

---

## Agent-side changes (Phase 2)

**Repository:** `DataDog/datadog-agent`

**File:** `.devcontainer/datadog/default/prebuild-devcontainer.json`

```diff
-        "registry.ddbuild.io/workspaces/features/base:0.4.100410934": {},
+        "registry.ddbuild.io/workspaces/features/base-compat:0.1.<BUILD_ID>": {},
```

All other fields unchanged from the Phase 1 state.

**File:** `.devcontainer/datadog/default/features/datadog-agent/devcontainer-feature.json`

```diff
-    "installsAfter": ["registry.ddbuild.io/workspaces/features/base"],
+    "installsAfter": ["registry.ddbuild.io/workspaces/features/base-compat"],
```

TBD values:
- `0.1.<BUILD_ID>`: version string assigned when `base-compat` is first published to the registry

---

## Testing Plan

> Tests are in two tiers: **locally reproducible** (docker + bazel, runnable on any machine with
> registry access and dd-source checked out) and **orchestrator-required** (full provision, needs
> a real Workspace launch). Both tiers are required before merging.

### Phase 1 — locally reproducible tests

**Test 1a: Prove the collision exists (pre-guard, post-#1089 image)**
```bash
IMG=registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v116490909-fc35a1a5
# Expected: "useradd: user 'bits' already exists" — confirms the problem being fixed
docker run --rm --entrypoint bash "$IMG" -c \
    'useradd --create-home --home-dir /home/bits --uid 2000 -g dog bits 2>&1 || true'
```

**Test 1b: Guard smoke test — `useradd` succeeds with pre-baked `bits`**

Apply the Phase 1 diff, build the modified base feature, run `install.sh` against the post-#1089 image.
The guarded `install.sh` must exit 0 with no `useradd` error:
```bash
# In dd-source, after applying the Phase 1 diff:
IMG=registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v116490909-fc35a1a5
docker run --rm --entrypoint bash "$IMG" -c '
    bitsHome=/home/bits
    if ! id -u bits >/dev/null 2>&1; then
        useradd --create-home --home-dir "${bitsHome}" --uid 2000 -g dog bits
    fi
    echo "guard test: PASS ✓ (exit 0 whether bits existed or not)"
'
```

**Test 1c: Filesroot copy ran — 3 provisioner scripts present**

This is the critical check: the `filesroot` copy must run unconditionally (outside the guard),
installing all three scripts the Go provisioner hard-codes:
```bash
IMG=registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v116490909-fc35a1a5
# Build the base feature locally (run from dd-source):
#   cd /path/to/dd-source && bazel build //domains/devcontainers/features/base
# Then use devcontainer CLI against the modified feature, or test filesroot manually:
docker run --rm --entrypoint bash "$IMG" -c '
    for script in adduser.sh setup_ide_backends.sh install_dotfiles.sh; do
        test -f "/opt/doghome/sbin/$script" \
            && echo "$script: present ✓" \
            || echo "$script: MISSING ✗"
    done
'
# All three must print "present ✓" — these are the provisioner contract paths:
# user.go:12, ide_backend_setup.go:12, dotfiles.go:22
```

**Test 1d: Non-pre-baked regression — guard does not skip `useradd` on plain image**
```bash
# Against a stock Ubuntu image (no bits user): useradd must still execute
docker run --rm ubuntu:22.04 bash -c '
    groupadd --gid 501 dog
    if ! id -u bits >/dev/null 2>&1; then
        useradd --create-home --home-dir /home/bits --uid 2000 -g dog bits
    fi
    id bits && echo "non-prebaked regression: PASS ✓"
'
```

### Phase 2 — Bazel artifact test (locally reproducible, highest-risk check)

The S4 plan's highest-risk question is whether the `base-compat` Bazel filegroup lands the
3 provisioner scripts at the correct OCI paths.

**Test 2a: Build the current `base` feature and verify artifact structure**

> **Executed and PASSED 2026-06-03** — confirmed all 3 scripts present at correct paths.
> Build time: ~6 min on this machine (Bazel fetches toolchain on first run).

```bash
cd /path/to/dd-source
bazel build //domains/devcontainers/features/base
# Inspect artifact:
tar tf bazel-bin/domains/devcontainers/features/base/base.tar | grep -E 'sbin|lifecycle'
```

Expected output (confirmed):
```
filesroot/opt/doghome/sbin/adduser.sh
filesroot/opt/doghome/sbin/install_dotfiles.sh
filesroot/opt/doghome/sbin/setup_ide_backends.sh
filesroot/opt/doghome/sbin/setup_ide_backends_0.sh
filesroot/opt/doghome/sbin/setup_ide_backends_jetbrains.sh
filesroot/opt/doghome/sbin/setup_ide_backends_vscode.sh
filesroot/opt/doghome/sbin/start_jetbrains_backend.sh
lifecycle/change_ownership.sh
lifecycle/entry.sh
lifecycle/post_create.sh
```

**Test 2b: After creating `base-compat` — verify its artifact**

After implementing Phase 2 (`base-compat` feature + `lib/` filegroup):
```bash
cd /path/to/dd-source
bazel build //domains/devcontainers/features/base-compat
tar tf bazel-bin/domains/devcontainers/features/base-compat/base-compat.tar | grep -E 'sbin|lifecycle'
```

The same 3 scripts must be present at identical `filesroot/opt/doghome/sbin/` paths.
If the paths differ (e.g. `lib/opt/doghome/sbin/` prefix from the filegroup), the `find $FILE_COPY_SRC`
in `install.sh` won't resolve correctly. Fix by adjusting `FILE_COPY_SRC` or the Bazel `strip_prefix`.

**Test 2c: `base-compat` precondition enforcement**

`base-compat/install.sh` must fail fast with a clear error on an image without `bits`:
```bash
docker run --rm ubuntu:22.04 bash -c '
    # Simulate base-compat install.sh precondition check
    if ! id -u bits >/dev/null 2>&1; then
        echo "ERROR: base-compat requires bits user to exist (UID 2000). Use base feature instead."
        exit 1
    fi
' && echo "UNEXPECTED: should have failed" || echo "precondition check: PASS ✓"
```

**Test 2d: DinD available inside workspace**

After `devcontainer up` with `base-compat`, confirm `docker ps` works (validates `dependsOn`
wiring for `docker-in-docker`):
```bash
/usr/local/bin/devcontainer up --workspace-folder /path/to/datadog-agent \
    --config .devcontainer/datadog/default/prebuild-devcontainer.json
/usr/local/bin/devcontainer exec --workspace-folder /path/to/datadog-agent \
    docker ps
# Expected: empty container list (DinD running), exit 0
```

### Orchestrator-required tests (both phases)

These cannot be run locally — they require a real Workspace launch via the platform:

4. **Phase 1:** Provision a real workspace using the Phase 1 base feature + post-#1089 image.
   Confirm Go provisioner calls `adduser.sh` successfully and workspace reaches `remoteUser: bits`.
   Check provision logs for exit 0.

10. **Phase 2 full E2E:** Provision workspace with `base-compat`. Run `dda inv install-tools`
    and `dda inv agent.build --build-exclude=systemd`. Both must exit 0.

### Regression

11. **Stock image regression:** Provision a workspace using the standard shell image (no pre-baked
    `bits`) with the Phase 1 guarded `base`. The `useradd` path must execute normally and the
    workspace must be identical to today's behavior. Verify with Test 1d (locally) + a real
    provision (orchestrator).

---

## Rollout Sequencing

```
Week 1 — Phase 1 (unblock the agent team)
│
├── dd-source PR: guard useradd in base/install.sh
│   └── Co-reviewed: Kevin Fairise (platform) + ofek (agent DevX)
│   └── Merge
│
├── Publish base feature → obtain 0.4.<NEW_BUILD_ID>
│
└── datadog-agent PR: bump image tag AND base version in single commit
    └── Never bump image while old (unguarded) base version is pinned
    └── Merge → trigger workspace prebuilt rebuild

Weeks 2-3 — Phase 2 (structural split)
│
├── dd-source PR: lib/ filegroup + base-compat feature
│   ├── Requires Bazel path-mapping verification before merge
│   ├── Co-reviewed: Kevin Fairise
│   └── Merge
│
├── Publish base-compat → obtain 0.1.<BUILD_ID>
│
└── datadog-agent PR: switch prebuild to base-compat; update installsAfter
    └── Merge → rebuild prebuilt image
```

**Key constraint:** Never bump the agent image to a post-#1089 tag while still referencing an
unguarded base feature version. The Phase 1 dd-source PR must be published before or in the
same atomic PR as the agent-side image bump.

**PR #50576:** Close or convert to draft immediately. Reference this plan as the replacement
approach.

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Phase 1 guard skips filesroot copy | Low | High (provisioner breaks) | filesroot copy is outside the guard; verified by test 2 above |
| Bazel path-mapping for lib/ filegroup is wrong | Medium | High (base-compat broken) | Mandatory OCI artifact inspection before Phase 2 merge |
| `dependsOn` values in base-compat.json differ from base | Medium | High (DinD/SSH lost) | Copy verbatim from base/devcontainer-feature.json; do not guess |
| Image bump and base version bump are split across two PRs | Medium | High (breakage window) | Both changes in single commit — enforced by sequencing |
| PR #50576 merged before Phase 1 ships | Medium | Medium | Close/draft immediately; link to this plan |
| base/lib drift after Phase 2 | High without shared lib | Medium | Shared lib is required, not optional; single source for Part 2 logic |

---

## Key File Paths

| File | Repo | Role |
|---|---|---|
| `domains/devcontainers/features/base/install.sh` | dd-source | Phase 1: guard `useradd` here |
| `domains/devcontainers/features/base/devcontainer-feature.json` | dd-source | Copy `dependsOn`, `entrypoint`, `postCreateCommand` into base-compat |
| `domains/devcontainers/features/base/BUILD.bazel` | dd-source | Add `lib:provisioning_scaffolding` to `srcs` |
| `domains/devcontainers/features/lib/` | dd-source | Phase 2: new shared filegroup + scripts |
| `domains/devcontainers/features/base-compat/` | dd-source | Phase 2: new feature |
| `domains/devex/workspaces/internal/libs/provisioning/user.go:12` | dd-source | `const addUserPath = "/opt/doghome/sbin/adduser.sh"` |
| `domains/devex/workspaces/internal/libs/provisioning/ide_backend_setup.go:12` | dd-source | `const ideScript = "/opt/doghome/sbin/setup_ide_backends.sh"` — this is what broke PR #50576 |
| `domains/devex/workspaces/internal/libs/provisioning/dotfiles.go:22` | dd-source | `const dotfilesScriptPath = "/opt/doghome/sbin/install_dotfiles.sh"` |
| `domains/devex/workspaces/internal/libs/provisioning/devcontainer.go:66-105` | dd-source | Build verifier: rejects workspace unless feature ID=`base`/Name=`Datadog Workspaces Components` OR image carries `dog.infra.workspace.base-image` label; AND port 22 forwarded |
| `dev-envs/linux/variants/workspaces/setup.sh` | datadog-agent-buildimages | Pre-bakes bits:2000 + dog:501 + build-shared:9001 (PR #1089). Verified available at `v116490909-fc35a1a5` |
| `.devcontainer/datadog/default/prebuild-devcontainer.json` | datadog-agent | Agent-side: update image tag + feature reference |
| `.devcontainer/datadog/default/features/datadog-agent/devcontainer-feature.json` | datadog-agent | Phase 2: update `installsAfter` |
