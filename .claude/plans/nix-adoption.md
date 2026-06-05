# Implementation Plan: Adopting Nix as Build Dependency Manager for `datadog-agent`

> Produced by Opus planning agent. Read and verify every TBD before executing.
> Target platforms: Linux (x86_64, aarch64) and macOS. Windows explicitly out of scope.
> Goal: from a fresh clone, `nix develop` passes the 15-item verification suite including `dda inv omnibus.build`.

---

## 0. Verified ground truth (from reading the repo)

| Fact | Source | Implication |
|---|---|---|
| `agent.build` builds rtloader via CMake `rtloader_make()` unless `exclude_rtloader=True` | `tasks/agent.py:101-110`; flag at `:81` | **Item 3 (`agent.build --build-exclude=systemd`) needs Go + C/C++ + Python-dev-headers + the rtloader CMake change all at once.** It does not exclude rtloader. |
| rtloader Python discovery is `find_package(Python3 COMPONENTS Interpreter Development)` with `CMP0094 NEW` (favor location) | `rtloader/three/CMakeLists.txt:2`, `rtloader/CMakeLists.txt:11-14` | Override knob is standard CMake: `-DPython3_ROOT_DIR`. No source edit strictly required, but we add an env-gated hint for robustness. |
| CMake options flow from `tasks/rtloader.py:make()` via `cmake_options` arg and `DD_CMAKE_TOOLCHAIN` env | `tasks/rtloader.py:36-68` | Inject Python path via env var without editing Python task code. |
| `agent.build` default path is CMake; Bazel is opt-in (`enable_bazel=False`) | `tasks/agent.py:90,104-110` | Nix targets the CMake rtloader path. Bazel path untouched. |
| `install-tools` = parallel `go install` of 10 Go tools + symlink bazelisk→bazel; cargo tools are a separate task | `tasks/install_tasks.py:14-89,114-122` | Nix provides the Go/cargo toolchains + writable GOBIN/CARGO_HOME; `install-tools` keeps running. |
| `golangci-lint` version is hard-pinned to `internal/tools/go.mod` and `check_tools_version` hard-fails on mismatch | `tasks/libs/common/check_tools_version.py:33-44,62-72`; `exit_on_error=True` | **Do NOT source golangci-lint from nixpkgs** — it will drift and break item 5. Must come from `go install` via `internal/tools/go.mod`. |
| Cargo tools pinned via `cargo install --locked` | `tasks/install_tasks.py:114-122` | Same drift risk. Keep in `install-tools`, not the flake. |
| omnibus = `bundle install` then `bundle exec omnibus build`; writes `/opt/datadog-agent` | `tasks/omnibus.py:46-90,125-144`; `omnibus/Gemfile` | Nix provides Ruby+bundler (non-root). Full build needs network + writable `/opt/datadog-agent`. Tight loop checks Ruby stage only (Slice 7). |
| `.go-version`=`1.25.10`, `.python-version`=`3.12`, `rust-toolchain.toml` has components only, no channel; `Cargo.toml` says `rust-version = "1.91"` | the four files | Flake reads `.go-version`/`.python-version` via `builtins.readFile`. `rust-toolchain.toml` must gain a `channel` line (§6 — file change). |
| dda pinned to `.dda/version`=`0.29.0` | `.dda/version` | Provide `dda` in the flake (via uv wrapper); see §1. |
| Flakes only see git-tracked files | Nix behavior | New `flake.nix`/`.envrc` are invisible until `git add -N`. Document in every slice runbook. |

---

## 1. The `flake.nix` structure

### Inputs
```nix
{
  inputs = {
    nixpkgs.url     = "github:NixOS/nixpkgs/<PINNED_REV>";  # TBD Note A
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay    = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
}
```

### Single-source version wiring
Inside the flake, read the existing version files so `flake.lock` is the source of truth for provenance while the repo files remain the source of truth for versions:
```nix
goVersion    = lib.strings.trim (builtins.readFile ./.go-version);     # "1.25.10"
pyVersionRaw = lib.strings.trim (builtins.readFile ./.python-version); # "3.12"
```

### Toolchains in the devShell

| Concern | Provider | Notes |
|---|---|---|
| **Go** | `pkgs.go` overridden to exactly `goVersion` | **TBD Note A:** verify pinned nixpkgs rev ships 1.25.10. If not, `go.overrideAttrs` with upstream tarball sha256 — mark as fill-in TBD. |
| **Rust** | `rust-overlay` `rust-bin.fromToolchainFile ./rust-toolchain.toml` | Honors components. **Requires `channel` added to `rust-toolchain.toml`** (§6). |
| **Python (dev)** | `pkgs.python312` (resolved from `pyVersionRaw`) | Must expose dev headers. Dev-only — distinct from omnibus embedded Python. |
| **Ruby + bundler** | `pkgs.ruby_3_X` + `pkgs.bundler` | **TBD Note B:** confirm Ruby minor expected by omnibus-ruby `datadog-5.5.0`; pin to that. Writable `GEM_HOME`/`BUNDLE_PATH` in shellHook. |
| **Host C/C++ toolchain** | `pkgs.stdenv.cc`, `pkgs.cmake`, `pkgs.gnumake`, `pkgs.pkg-config` | For rtloader CMake build. Release cross-compilers are OUT OF SCOPE. |
| **dda CLI** | `pkgs.uv` + shellHook `uv tool install dda==<.dda/version>` into writable prefix | **TBD Note C:** confirm dda's PyPI index name and install mechanism. |
| **System libs for cgo/omnibus** | `pkgs.openssl`, `pkgs.zlib`, `pkgs.libffi`, etc. | **TBD Note D:** enumerate empirically from clean-room linker/CMake errors. Start minimal, grow. |
| **Misc tools** | `pkgs.git`, `pkgs.coreutils`, `pkgs.patchelf` (Linux), `pkgs.curl`, `pkgs.cacert` | patchelf used by rtloader RPATH logic and omnibus rpath_edit. |

### shellHook responsibilities
```bash
export GOBIN="$PWD/.gobin"
export GOMODCACHE="$PWD/.gomodcache"
export CARGO_HOME="$PWD/.cargo-home"
export GEM_HOME="$PWD/.gem"
export BUNDLE_PATH="$PWD/.bundle"
export PATH="$GOBIN:$CARGO_HOME/bin:$PATH"
export SSL_CERT_FILE="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
# Rtloader Python hint (§3)
export DD_RTLOADER_PYTHON3_ROOT="${python}"
echo "Nix dev shell ready — Go $(go version), Rust $(rustc --version | cut -d' ' -f2), Python $(python3 --version)"
```

### What the flake does NOT pin
golangci-lint, gotestsum, mockery, dd-rust-license-tool, cargo-deny, and the other `install-tools` binaries are **not** nixpkgs packages. They are installed by the thin `install-tools` task from their pinned manifests.

---

## 2. The `rtloader/` CMake changes

### The change
Add an **env-gated hint** at the top of `rtloader/three/CMakeLists.txt` *before* `find_package`, so a Nix-provided Python is used when present, while the Conda/default path is completely unchanged when the env var is absent:

```cmake
# Allow an external dependency manager (e.g. Nix) to point CMake at a specific
# Python without disturbing the default discovery used by Conda-based builds.
if(DEFINED ENV{DD_RTLOADER_PYTHON3_ROOT})
    set(Python3_ROOT_DIR "$ENV{DD_RTLOADER_PYTHON3_ROOT}"
        CACHE PATH "Python root provided by Nix devShell")
    set(Python3_FIND_STRATEGY LOCATION)
endif()

find_package(Python3 COMPONENTS Interpreter Development)
```

### Why this design
- **Conda path untouched:** when `DD_RTLOADER_PYTHON3_ROOT` is unset (any non-Nix user, CI buildimage), the block is skipped entirely.
- **No task edits required:** the env var is consumed by CMake directly; `tasks/rtloader.py` and `tasks/agent.py` need no changes.
- `Python3_FIND_STRATEGY LOCATION` reinforces the existing `CMP0094 NEW`.

### Validation note
Run `dda inv rtloader.clean` before re-building after this change — CMake caches the discovered Python path and the cache must be invalidated when switching from Conda to Nix.

---

## 3. The `dda inv install-tools` transition

**Decision: it stays and becomes *thin*, not a no-op.**

- The flake provides the Go toolchain, Rust toolchain, and writable `GOBIN`/`CARGO_HOME`.
- `install-tools` continues to `go install` the 10 pinned tools into `$GOBIN` and `cargo install --locked` the two pinned cargo tools into `$CARGO_HOME/bin`.
- **No mandatory code change to `tasks/install_tasks.py`.** Optional change: short-circuit `install_shellcheck`/`install_devcontainer_cli` when running under Nix (provide those from the flake instead, since they write to `/usr/local/bin` which is not writable). Mark as optional — validate in Slice 2.

Net effect: inside `nix develop`, `dda inv install-tools` exits 0 as non-root, writing to user-owned dirs under the repo, without needing `build-shared` or root.

---

## 4. The iteration loop

### Mechanics (applied to every slice)
- **Worktree per slice:** `git worktree add ../nix-slice-N nix/slice-N`. Each slice's branch is isolated; merge to integration branch `nix/adopt` on pass.
- **`git add -N` required:** after creating `flake.nix`/`.envrc`/`flake.lock`, run `git add -N <files>` (intent-to-add) immediately. Without this, `nix develop` errors with "path does not exist in Nix store."
- **Container clean-room with warm store:** repo is cloned fresh every run (the "fresh clone" contract); `/nix/store` persists across runs via a Docker named volume so iterations don't re-download the full toolchain each time.
- **Per-slice acceptance criteria are narrow** — the full 15-item suite runs only at Slice 8.

### Slice ordering (corrected: Python and host C before first `agent.build`)

The critical correction from the planning process: `agent.build --build-exclude=systemd` does NOT exclude rtloader. So it requires Go + C/C++ + Python-dev-headers + the rtloader CMake change simultaneously. Python and host-C must come before the first `agent.build`.

| Slice | Branch | Implements | Acceptance check (runs in clean-room container) |
|---|---|---|---|
| **0 — Bootstrap** | `nix/slice-0` | `flake.nix` (shell entry only), `.envrc`, `flake.lock`, `.gitignore` additions | `nix develop --command true` exits 0; `direnv allow` works. |
| **1 — Go toolchain** | `nix/slice-1` | Nix Go pinned to `.go-version`; writable `GOBIN`/`GOMODCACHE` | `go version` == 1.25.10; pure-Go build succeeds (e.g. `go build ./pkg/util/log/...`). **Not** `agent.build` yet. |
| **2 — Dev tools** | `nix/slice-2` | Writable GOBIN/CARGO_HOME wiring; optional install_tasks.py edits | `dda inv install-tools` exits 0; `golangci-lint version` matches `internal/tools/go.mod`. |
| **3 — Rust toolchain** | `nix/slice-3` | `rust-overlay` + `channel` in `rust-toolchain.toml` | `rustc --version` matches; `cargo check --workspace` succeeds; components present. |
| **4 — Python + host C** | `nix/slice-4` | `python312` with dev headers; gcc/clang, cmake, make, pkg-config | `python3-config --includes` resolves to Nix prefix; trivial CMake `find_package(Python3 Development)` smoke compiles. |
| **5 — rtloader CMake** | `nix/slice-5` | Edit `rtloader/three/CMakeLists.txt` (§2); `DD_RTLOADER_PYTHON3_ROOT` in shellHook | `dda inv rtloader.clean && dda inv rtloader.make` succeeds; assert resolved `Python3_EXECUTABLE` is under `/nix/store`. |
| **6 — Agent build** | `nix/slice-6` | No new code; proves Go+Python+C+rtloader compose | **Item 3:** `dda inv agent.build --build-exclude=systemd` exits 0. |
| **7 — Ruby + omnibus** | `nix/slice-7` | Nix Ruby+bundler; writable `GEM_HOME`/`BUNDLE_PATH` | Tight check: `cd omnibus && bundle install && bundle exec omnibus --version` succeeds as non-root. Full build (`dda inv omnibus.build`) gated behind `--include-slow`. |
| **8 — Full clean-room** | `nix/adopt` (merged) | Nothing new; harness wiring | Fresh container, fresh clone, warm store: all 15 items, per-command pass/fail report. |

---

## 5. The verification harness

Two new files: `tasks/nix-verify.sh` and `Dockerfile.nix-verify`.

### `Dockerfile.nix-verify`
```dockerfile
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y curl git xz-utils sudo
# Install Nix (Determinate Systems installer, flakes enabled)
RUN curl -sSfL https://install.determinate.systems/nix | sh -s -- install linux \
    --init none --no-confirm
ENV PATH="/nix/var/nix/profiles/default/bin:$PATH"
# Warm store is mounted at runtime via named volume (-v nix-store:/nix)
WORKDIR /repo
ENTRYPOINT ["nix", "develop", "--command", "bash", "tasks/nix-verify.sh"]
```

### `tasks/nix-verify.sh` contract
1. Accepts `--suite=<slice|all>` and `--include-slow` flags.
2. Runs each item in the suite, captures exit code + duration.
3. Prints `PASS [Ns] <command>` or `FAIL [Ns] <command>` per item.
4. Exits non-zero if any required item failed.
5. Items 1-15 are the 15-item verification suite; omnibus full build is gated behind `--include-slow`.
6. eBPF/system-probe/GPU/Windows/AWS/E2E items are explicitly asserted absent.

### Run command
```bash
# Build image (once):
docker build -f Dockerfile.nix-verify -t dd-agent-nix-verify .

# Run (fresh clone, warm store):
docker run --rm \
  -v nix-store-cache:/nix \
  -v "$PWD":/repo:ro \
  dd-agent-nix-verify --suite=all
```

---

## 6. Concrete file changes

| File | New/Modified | Change | Why |
|---|---|---|---|
| `flake.nix` | **New** | devShell per §1 | Single entry point for `nix develop` |
| `flake.lock` | **New (generated)** | `nix flake lock` output | Pins nixpkgs + rust-overlay + flake-utils revs |
| `rust-toolchain.toml` | **Modified** | Add `channel = "1.91.0"` under `[toolchain]` — **TBD: confirm exact patch** | `rust-overlay`'s `fromToolchainFile` requires a channel |
| `rtloader/three/CMakeLists.txt` | **Modified** | Env-gated `Python3_ROOT_DIR` block before `find_package` (§2) | Point rtloader at Nix Python without breaking Conda |
| `.envrc` | **New** | `use flake` | direnv auto-enters the shell on `cd` |
| `.gitignore` | **Modified** | Add `/.gobin/`, `/.cargo-home/`, `/.gem/`, `/.bundle/`, `/.gomodcache/`, `/.direnv/`, `result`, `result-*` | Writable toolchain dirs and Nix build symlinks must not be committed |
| `tasks/nix-verify.sh` | **New** | 15-item verification harness (§5) | Runs the suite per slice and at Slice 8 |
| `Dockerfile.nix-verify` | **New** | Clean-room container (§5) | Reproducible fresh-clone verification |
| `tasks/install_tasks.py` | **Modified (optional, Slice 2)** | Optionally short-circuit `install_shellcheck`/`install_devcontainer_cli` in Nix shell | Avoid `/usr/local/bin` writes that don't work non-root |

---

## 7. What does NOT change

Explicitly untouched:
- All Go source, `go.mod`/`go.sum`, build tags, `internal/tools/go.mod`
- All Bazel files (`MODULE.bazel`, `BUILD.bazel`, `.bazelrc`, `WORKSPACE`). Nix manages toolchains outside Bazel; Bazel keeps its own.
- All `tasks/` invoke logic except the two surgical edits above
- `omnibus/*.rb`, `omnibus/config/`, `omnibus/Gemfile` — Gemfile reads `OMNIBUS_RUBY_VERSION` from env; Nix supplies Ruby+bundler and writable gem dirs
- The embedded/agent-shipped Python (omnibus output — separate concern from dev Python)
- CI pipeline structure (`.gitlab-ci.yml`, GitHub Actions, buildimages). Swapping CI images is a **follow-on project**.
- Windows CI/dev — stays on current buildimages approach
- Release cross-compilation toolchain (glibc-targeting cross GCC) — stays in buildimages

---

## 8. Open TBDs (resolve before executing each slice)

| TBD | Where it matters | Resolution approach |
|---|---|---|
| **Note A:** Does pinned nixpkgs rev ship Go 1.25.10? | Slice 1 | Check `nix eval nixpkgs#go.version` on candidate rev; if not, `go.overrideAttrs` with upstream tarball sha256 |
| **Note B:** Exact Ruby minor for omnibus-ruby `datadog-5.5.0` | Slice 7 | Check `omnibus/Gemfile.lock` for `omnibus-ruby` Ruby constraint |
| **Note C:** dda PyPI index name / install mechanism for pinning 0.29.0 | Slice 0 | Check internal package index or `ddtool` docs |
| **Note D:** Full system-library closure (cgo + omnibus linker deps) | Slices 6-7 | Grow empirically from clean-room `ldd`/CMake errors; add each missing package to the flake |
| **Rust channel exact patch** (1.91.0 vs 1.91.x) | Slice 3 | Confirm with team; update `rust-toolchain.toml` |
| **Slice 2 optional install_tasks.py edits** | Slice 2 | Validate whether shellcheck/devcontainer-cli cause failures; add short-circuit only if needed |
