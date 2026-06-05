# Full S1 Nix Vision — Executable Implementation Plan

> Produced by Opus planning agent. Resolve every TBD (especially TBD-0) before executing.
> Extends `.claude/plans/nix-adoption.md` (dev-shell, already implemented) to cover the release toolchain.

---

## 0. Ground-truth reconciliation (READ THIS FIRST)

The repo is **mid-migration** between two release toolchain paths. The plan's scope depends on which one CI actually uses — **resolve TBD-0 before anything else**.

| Surface | Buildimage path (shell scripts, current default `enable_bazel=False`) | Bazel path (transitional, `enable_bazel=True`) |
|---|---|---|
| Embedded Python | Conda/Miniforge `4.9.2-7`, `PY3_VERSION=3.12.6` at `/opt/dd/conda` | Bazel `@cpython` http_archive `3.13.13` (`MODULE.bazel`) |
| Cross-compilers | crosstool-ng `1.26.0` → `/opt/toolchains/{x86_64,aarch64}`, binaries `{arch}-unknown-linux-gnu-gcc`, glibc `2.17`/`2.23` | Bazel `gcc_toolchain` consuming prebuilt S3 tarball |
| macOS cross | osxcross commit `e6ab3fa7`, MacOSX SDK `14.0` | `toolchains_llvm` + `macos_sdk` `15.5` |

**Verified contracts the Nix output must satisfy:**
- `tasks/omnibus.py:489-503,613-621`: sets `DD_CC`/`DD_CXX` to `x86_64-unknown-linux-gnu-gcc` / `aarch64-unknown-linux-gnu-g++` — these must be on PATH
- `tasks/agent.py:104-107`: when `enable_bazel=False`, `embedded_path`/`python_home_3` come from the ambient Conda install baked into the image
- `omnibus/config/software/python3.rb`: on the Bazel path, runs `bazelisk run @cpython//:install` — **no `PYTHON_HOME` env var** (contradicts the task premise)

**Architectural decision — Nix *feeds* Bazel and *replaces* the buildimage shell scripts:**
- Nix produces the Docker image (replaces `linux/Dockerfile`)
- Inside the image, Nix provides ctng-equivalent cross-compilers on PATH (satisfying `DD_CC`) and a Conda-equivalent embedded Python
- For the Bazel path: Nix can later produce the S3 toolchain tarball `gcc_toolchain` consumes (Phase 2 — Section 3.4)
- This keeps `flake.lock` as the image source of truth while leaving Bazel's hermeticity intact

**TBD-0 (blocking, reorders everything):** Confirm whether release CI uses `enable_bazel=True` or `False`.
```bash
grep -r 'enable.bazel\|enable_bazel' .gitlab-ci.yml tasks/
```
If CI is already on Bazel: Section 2 (embedded Python) priority drops; cross-compilers (Section 3) are the critical path.

---

## 1. Flake structure extension

Extend `/flake.nix`'s existing `eachDefaultSystem` block. Add a `packages` attrset; keep `devShells.default` unchanged. Move all release-toolchain version pins into a single `buildimageVersions` attrset that mirrors `docker-bake.hcl`:

```nix
buildimageVersions = {
  py3            = "3.12.6";         # PY3_VERSION (Conda path)
  ctng           = "1.26.0";         # CTNG_VERSION
  glibcX86       = "2.17";           # GLIBC_VERSION (amd64 host)
  glibcArm       = "2.23";           # CROSS_GLIBC_VERSION (amd64 host)
  mold           = "2.40.4";
  protobuf       = "34.0";
  bundler        = "2.4.20";
  osxcrossCommit = "e6ab3fa7423f9235ce9ed6381d6d3af191b46b59";
  macosSdk       = "14.0";
};
```

New output structure — one Nix file per gap:
```nix
packages = {
  embeddedPython  = import ./nix/embedded-python.nix   { inherit pkgs buildimageVersions; };
  crossToolchains = import ./nix/cross-toolchains.nix  { inherit pkgs buildimageVersions; };
  osxcross        = import ./nix/osxcross.nix          { inherit pkgs buildimageVersions; };
  dockerImage     = import ./nix/docker-image.nix      { inherit pkgs self buildimageVersions; };
};
```

**Also fix the existing devShell macOS gap** (currently breaks `nix develop` on Darwin):
```nix
# Wrap Linux-only inputs
] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
  pkgs.systemd.dev
  pkgs.patchelf
];
```

---

## 1b. `devShells.release` — local release-quality builds

Section 1 keeps `devShells.default` unchanged — but that shell intentionally lacks the cross-compilers and embedded Python, so a developer running `nix develop` **cannot** produce a release-floor binary locally. The release toolchain only existing inside `packages.dockerImage` means the glibc floor is verifiable in CI but not on a laptop. `devShells.release` closes that gap.

**Requirement:** developers must be able to run test builds locally using the Nix-managed release toolchain, and the resulting binaries must have the same glibc floor as CI release artifacts.

### The `devShells.release` definition

A second devShell that extends `devShells.default` via `inputsFrom` and layers the release-toolchain packages on top:

```nix
devShells.release = pkgs.mkShell {
  name = "datadog-agent-release";
  inputsFrom = [ devShells.default ];
  buildInputs = [
    crossToolchains.x86_64
    crossToolchains.aarch64
    embeddedPython
  ] ++ pkgs.lib.optionals pkgs.stdenv.isDarwin [ osxcross ];
  shellHook = devShells.default.shellHook + ''
    export PATH="${crossToolchains.x86_64}/x86_64/bin:${crossToolchains.aarch64}/aarch64/bin:$PATH"
    export PYTHON_HOME_3="${embeddedPython}"
    export EMBEDDED_PYTHON="${embeddedPython}"
    echo "  Release toolchain: $(x86_64-unknown-linux-gnu-gcc --version 2>/dev/null | head -1)"
    echo "  Embedded Python:   $(${embeddedPython}/bin/python3 --version 2>/dev/null)"
  '';
};
```

### How a developer uses it

```bash
nix develop .#release
dda inv agent.build    # produces a glibc-2.17-floor binary
dda inv omnibus.build  # produces a correct release artifact with Nix-built embedded Python
```

### Relationship between the two shells

| Shell | Purpose | Toolchain | First-run cost |
|---|---|---|---|
| `devShells.default` | Inner loop — fast iterate/test/build. Daily driver. | Host glibc, no cross-compilers | Cheap (no ctng) |
| `devShells.release` | Validate release artifacts locally before CI. | Nix cross-compilers + embedded Python, glibc 2.17/2.23 floor | ~30-60 min first ctng build, cached thereafter |

Neither replaces the other: `default` is the daily driver; `release` is opt-in for proving an artifact matches the CI release floor.

### `PYTHON_HOME_3` / `embedded_path` wiring

The `devShells.release` shellHook exports `EMBEDDED_PYTHON` pointing at the Nix embedded Python derivation. `tasks/agent.py:104-107` controls `embedded_path`/`python_home_3` when `enable_bazel=False` — currently these are function parameters, not env vars.

**TBD-11 (blocking for `devShells.release`):** Extend `tasks/agent.py` or `dda inv agent.build` to accept `EMBEDDED_PYTHON` as the `python_home_3` override when `enable_bazel=False`. Verify the exact wiring point by reading `tasks/agent.py:104-117` and `tasks/rtloader.py` — confirm whether the override is an existing CLI flag (`--python-home-3`), an existing env var, or needs to be added. (Grounding note: at `tasks/agent.py:108-117`, the non-Bazel branch calls `rtloader_make(ctx, install_prefix=embedded_path, ...)` and passes `python_home_3` into `get_build_flags` as a function parameter — so the override requires plumbing an env-var reader into this call site.)

---

## 2. Gap 1 — Embedded Python (`packages.embeddedPython`)

### Discovery mechanism (corrected)

The task's assumed `PYTHON_HOME`/`OMNIBUS_PYTHON_DIR` env var **does not exist** in the current omnibus:
- `omnibus/config/software/python3.rb` on the Bazel path: runs `bazelisk run @cpython//:install`
- On the buildimage (`enable_bazel=False`) path: omnibus finds Python via the ambient Conda prefix baked into the image

**Target: replace the Conda prefix in the image with a Nix-built CPython.** On the buildimage path, `embedded_path`/`python_home_3` default to the Conda prefix; the Nix image places the Nix-built Python at that same prefix path, or `agent.build` is invoked with explicit `--python-home-3=$(nix build .#embeddedPython --print-out-paths)`.

### The derivation (`nix/embedded-python.nix`)

Build CPython `3.12.6` (matching `PY3_VERSION`, distinct from the dev-shell 3.12 and the Bazel 3.13.13):

```nix
{ pkgs, buildimageVersions }:
(pkgs.python312.override {
  openssl = pkgs.openssl_3;  # TBD-1: verify exact version from Conda ddpy3
  enableOptimizations = true; # match Conda PGO build
  reproducibleBuild = false;  # PGO is non-deterministic; see Tier 5 caveat
}).overrideAttrs (old: {
  version = buildimageVersions.py3;
  src = pkgs.fetchurl {
    url = "https://www.python.org/ftp/python/3.12.6/Python-3.12.6.tgz";
    hash = "TBD-2";  # nix-prefetch-url the tarball
  };
  configureFlags = (old.configureFlags or []) ++ [
    "--enable-shared"    # libpython3.12.so required for rtloader dlopen
    "--with-system-ffi"
  ];
})
```

**TBD-1 (blocking):** Get the exact openssl version and configure args from a running buildimage:
```bash
docker run --rm registry.ddbuild.io/ci/datadog-agent-buildimages/linux:<tag> \
  bash -lc 'conda activate ddpy3 && python3 -c "
import ssl,sysconfig
print(ssl.OPENSSL_VERSION)
print(sysconfig.get_config_var(\"CONFIG_ARGS\"))
"'
```

**TBD-2:** `nix-prefetch-url https://www.python.org/ftp/python/3.12.6/Python-3.12.6.tgz`

**TBD-3 (decision):** Bit-identity with Conda is impossible (PGO profiles differ). The acceptance criterion is functional: same version + ssl version + shared library linkage + import test suite passing.

---

## 3. Gap 2 — Cross-compilers (`packages.crossToolchains`)

### Decision: crosstool-ng as a Nix derivation (Approach A)

**Rationale for choosing crosstool-ng over `pkgsCross`:**
1. `DD_CC` contract requires exact triple `{arch}-unknown-linux-gnu-gcc` — `pkgsCross` produces differently-named wrappers
2. glibc 2.17 (2012) won't bootstrap cleanly against modern nixpkgs; overlaying it into `pkgsCross` requires patching the whole cross-stdenv — effectively reinventing ctng poorly
3. The ctng derivation output can also feed Bazel's `gcc_toolchain` (Section 3.4), unifying both paths

### The derivation (`nix/cross-toolchains.nix`)

```nix
{ pkgs, buildimageVersions }:
let
  mkToolchain = { targetArch, glibcVersion, configFile }:
    pkgs.stdenv.mkDerivation {
      name = "dd-ctng-${targetArch}-glibc${glibcVersion}";
      src = pkgs.fetchurl {
        url = "http://crosstool-ng.org/download/crosstool-ng/crosstool-ng-${buildimageVersions.ctng}.tar.xz";
        hash = "TBD-4";
      };
      nativeBuildInputs = with pkgs; [
        gperf help2man bison flex texinfo ncurses which python3
        gnumake gcc autoconf automake libtool wget rsync unzip
      ];
      patches = [ ./ctng.patch ];  # TBD-6: copy from buildimages repo
      buildPhase = ''
        ./configure --enable-local && make -j$NIX_BUILD_CORES
        cp ${configFile} .config
        export CT_ALLOW_BUILD_AS_ROOT_SURE=yes
        ./ct-ng upgradeconfig
        ./ct-ng build
      '';
      installPhase = ''
        mkdir -p $out
        mv $HOME/x-tools/${targetArch}-unknown-linux-gnu $out/${targetArch}
      '';
    };
in {
  x86_64  = mkToolchain { targetArch = "x86_64";  glibcVersion = buildimageVersions.glibcX86; configFile = ./ctng-config-x86_64; };
  aarch64 = mkToolchain { targetArch = "aarch64"; glibcVersion = buildimageVersions.glibcArm; configFile = ./ctng-config-aarch64; };
}
```

**TBD-5 (critical — ctng sandbox issue):** `ct-ng build` downloads sources at build time. Nix's sandbox forbids network access.

- **5a (correct, hermetic):** Pre-fetch all ctng source tarballs (GCC, glibc, binutils, gmp, mpfr, etc.) as `fetchurl` inputs; set `CT_LOCAL_TARBALLS_DIR` + `CT_FORBID_DOWNLOAD=y`. Enumerate the manifest by running ctng once in a buildimage container: `ls .build/tarballs/` after a failed build.
- **5b (impure, stopgap):** Use `__noChroot = true` to allow network. Unblocks Tier 2 verification fast; replace with 5a before shipping.

Use 5b to unblock, 5a for the final form.

**TBD-6 (blocking):** Fetch the ctng config files and patch from buildimages:
```bash
gh api repos/DataDog/datadog-agent-buildimages/contents/linux/ | jq '.[] | .name'
# Look for .config, ctng.patch, config files referenced in the Dockerfile COPY lines
```

**TBD-4:** `nix-prefetch-url http://crosstool-ng.org/download/crosstool-ng/crosstool-ng-1.26.0.tar.xz`

### PATH / contract satisfaction

In the Docker image, expose both toolchain bin dirs on PATH and create the `/opt/toolchains` symlinks that `tasks/kernel_matrix_testing/compiler.py:288-300` hardcodes:
```
/opt/toolchains/x86_64 → $out/x86_64
/opt/toolchains/aarch64 → $out/aarch64
```

### 3.4 Feeding Bazel (Phase 2 — later)

Produce the S3 tarball `gcc_toolchain` already consumes:
```nix
pkgs.runCommand "gcc-toolchain-tarball" {} ''
  tar czf $out/datadog_agent_cc_toolchain_ubuntu_22_gcc_11.4.0_x86_64.tar.gz \
    -C ${crossToolchains.x86_64} .
''
```
Upload to S3; update `MODULE.bazel` url+sha256. This unifies both toolchain paths under `flake.lock`. Mark as Phase 2 — not required for buildimage replacement.

---

## 4. Gap 3 — osxcross (`packages.osxcross`)

Lowest priority (macOS CI builds; Bazel path already on `toolchains_llvm` + SDK 15.5).

```nix
# nix/osxcross.nix
pkgs.osxcross.overrideAttrs (old: {
  src = pkgs.fetchFromGitHub {
    owner = "tpoechtrager"; repo = "osxcross";
    rev = buildimageVersions.osxcrossCommit;
    hash = "TBD-7";
  };
  SDK = pkgs.requireFile {
    name = "MacOSX14.0.sdk.tar.xz";
    sha256 = "TBD-8a";   # from docker-bake.hcl MACOSX_SDK_SHA256
  };
})
```

**TBD-8 (blocking — legal):** The MacOSX 14.0 SDK is non-redistributable. Must be supplied via internal binary cache or `nix-store --add-fixed`. Document that operators must provide it; it cannot be auto-fetched by a public derivation.

**TBD-7:** `gh api repos/DataDog/datadog-agent-buildimages/contents/linux/scripts/osxcross.sh` to find the commit hash and SDK sha256.

---

## 5. Docker image output (`packages.dockerImage`)

```nix
# nix/docker-image.nix — guard Linux-only
{ pkgs, self, buildimageVersions }:
assert pkgs.stdenv.isLinux;
pkgs.dockerTools.buildLayeredImage {
  name = "registry.ddbuild.io/ci/datadog-agent-buildimages/linux";
  # Deterministic tag from flake.lock hash — replaces manually-bumped image tags
  tag = builtins.substring 0 12 (builtins.hashFile "sha256" ../flake.lock);
  maxLayers = 100;
  # Ordered cold→hot for layer cache reuse:
  contents = with pkgs; [
    crossToolchains.x86_64 crossToolchains.aarch64   # largest, most stable
    embeddedPython                                    # changes on py bump
    osxcross
    go_1_25 rustPkg ruby_3_3 bundler cmake mold
    protobuf openssl zlib git cacert bashInteractive coreutils
  ];
  config.Env = [
    "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
  ];
  extraCommands = ''
    mkdir -p opt/toolchains
    ln -s ${crossToolchains.x86_64}/x86_64  opt/toolchains/x86_64
    ln -s ${crossToolchains.aarch64}/aarch64 opt/toolchains/aarch64
  '';
}
```

**TBD-9:** Wrap `packages.dockerImage` in `lib.optionalAttrs pkgs.stdenv.isLinux { ... }` at the flake output level so `nix flake check` passes on Darwin.

---

## 6. Verification strategy (container-based, iterable)

Extend `tasks/nix-verify.sh` (existing) with a `--suite=release` flag. Each tier runs in `Dockerfile.nix-verify` with a warm Nix store volume. On fail: fix the relevant `nix/*.nix`, run `nix build .#<pkg>` (Nix caching rebuilds only the changed derivation). Iteration target: **Tier 4 green**.

### Tier 1 — Toolchain presence (~2 min) — extend existing suite
```bash
run_check "x86_64 cross-gcc present"   bash -c 'command -v x86_64-unknown-linux-gnu-gcc'
run_check "aarch64 cross-gcc present"  bash -c 'command -v aarch64-unknown-linux-gnu-gcc'
run_check "embedded Python 3.12.6"     bash -c 'nix run .#embeddedPython -- --version | grep -q "3.12.6"'
run_check "mold linker present"        mold --version
run_check "protoc present"             protoc --version
```

### Tier 2 — glibc floor check (~10 min) — THE core release correctness gate
```bash
# Build agent with Nix cross-compilers
DD_CC=x86_64-unknown-linux-gnu-gcc \
DD_CXX=x86_64-unknown-linux-gnu-g++ \
  dda inv agent.build --build-exclude=systemd

# Assert glibc floor:
MAX_GLIBC=$(objdump -T bin/agent/agent \
  | grep -oP 'GLIBC_\K[0-9.]+' | sort -V | tail -1)
# PASS iff MAX_GLIBC <= 2.17 (x86_64) / <= 2.23 (aarch64)
python3 -c "
import sys
from packaging.version import Version
max_v = Version('$MAX_GLIBC')
limit = Version('2.17')  # aarch64: 2.23
sys.exit(0 if max_v <= limit else 1)
"
```

> **Runnable locally:** when in `nix develop .#release`, this exact Tier 2 check runs on a laptop, not just in CI — same `objdump -T | grep GLIBC_` gate. It is the local answer to "does this binary actually hit glibc 2.17?" and is the earliest local proof the release shell is wired correctly.

### Tier 3 — Embedded Python correctness (~5 min)
```bash
PY=$(nix build .#embeddedPython --print-out-paths)
$PY/bin/python3 --version | grep -q "3.12.6"
ldd $PY/lib/libpython3.12.so                         # must resolve
$PY/bin/python3 -c 'import ssl; v=ssl.OPENSSL_VERSION; print(v)'  # == TBD-1 value
$PY/bin/python3 -m test -j4 test_ssl test_ctypes test_importlib    # embedding subset
```

### Tier 4 — Full omnibus artifact (~45 min, `--include-slow`)
```bash
# Run inside the Nix-produced Docker image:
dda inv omnibus.build

# Assert on the produced deb:
docker run --rm ubuntu:22.04 bash -c "
  dpkg -i /artifacts/datadog-agent_*.deb
  /opt/datadog-agent/bin/agent/agent version
  /opt/datadog-agent/embedded/bin/python3 --version | grep 3.12.6
  objdump -T /opt/datadog-agent/bin/agent/agent \
    | grep -oP 'GLIBC_\K[0-9.]+' | sort -V | tail -1
  # must print <= 2.17
"
```

### Tier 5 — Reproducibility (aspirational)
```bash
nix build .#dockerImage --rebuild   # build twice, compare output hashes
```
Note: PGO in `embeddedPython` breaks bit-identity. Real bar is functional equivalence (same glibc floor, Python version, ssl version). Document explicitly.

---

## 7. Buildimages migration

Files that change in `datadog-agent-buildimages`:

| File | Change |
|---|---|
| `linux/Dockerfile` | Replaced by `nix build .#dockerImage` invocation in CI |
| `docker-bake.hcl` | Version variables move into `flake.nix`'s `buildimageVersions`; file becomes a thin push wrapper |
| `linux/scripts/ctng.sh` | Obsoleted by `nix/cross-toolchains.nix` |
| `linux/scripts/python.sh` | Obsoleted by `nix/embedded-python.nix` |
| `linux/scripts/osxcross.sh` | Obsoleted by `nix/osxcross.nix` |

**Sequencing (atomic switchover):**
1. Build `packages.dockerImage` in parallel with the existing Dockerfile (push under `-nix` tag suffix)
2. Run Tiers 1-4 against the Nix image; run existing buildimage test suite against both
3. Cross-check: run `dda inv omnibus.build` from both images, compare glibc floors / Python versions
4. When Tier 4 passes and artifacts match: flip `CI_IMAGE_LINUX` in `.gitlab-ci.yml` to the Nix image digest (one commit). Keep old Dockerfile one release for rollback.

---

## 8. Developer flow changes

| Change | Detail |
|---|---|
| `docs/public/setup/manual.md` | Becomes: "install Nix, run `nix develop`" |
| `.gitlab-ci.yml` `CI_IMAGE_LINUX` | Replace image tag with Nix-produced digest |
| `dda inv update-go` task | Remove — `flake.lock` replaces `.go-version` ↔ buildimage sync |
| macOS dev shell | Add `lib.optionals stdenv.isLinux` guard (already noted in Section 1) |

**TBD-10:** Confirm task name and ensure `docker-bake.override.json` Go handling is also retired.

---

## 9. Implementation sequence

1. **Resolve TBD-0** — which path does release CI actually use? Reorders everything.
2. Fix devShell macOS guard + `buildimageVersions` attrset (low risk, immediate)
3. **Gap 2 cross-compilers** — Tier 2 gate. Resolve TBD-4/5/6. Use 5b to unblock fast.
4. **Gap 1 embedded Python** — Tier 3 gate. Resolve TBD-1/2.
4b. **Wire `devShells.release` and verify Tier 2 locally** — earliest point a developer can prove the release shell works (`nix develop .#release` → `dda inv agent.build` → objdump glibc-floor check). Resolve TBD-11.
5. **`packages.dockerImage`** + Tier 1 in-image assertions
6. **Tier 4 omnibus** — the iteration target
7. **Gap 3 osxcross** — lowest priority. Resolve TBD-7/8 SDK legal question.
8. Buildimages migration + dev-flow changes (Section 7-8)
9. Phase 2: feed Bazel the Nix-built toolchain tarball + cpython (Section 3.4)

---

## 10. Open TBDs

| TBD | Blocks | Resolution |
|---|---|---|
| **TBD-0** | Everything | `grep enable.bazel .gitlab-ci.yml tasks/agent.py` — does release CI use Bazel or shell-script path? |
| **TBD-1** | Section 2 | Run buildimage container: `conda activate ddpy3 && python3 -c "import ssl,sysconfig; print(ssl.OPENSSL_VERSION, sysconfig.get_config_var('CONFIG_ARGS'))"` |
| **TBD-2** | Section 2 | `nix-prefetch-url https://www.python.org/ftp/python/3.12.6/Python-3.12.6.tgz` |
| **TBD-4** | Section 3 | `nix-prefetch-url http://crosstool-ng.org/download/crosstool-ng/crosstool-ng-1.26.0.tar.xz` |
| **TBD-5** | Section 3 | ctng sandbox strategy: enumerate tarball manifest from buildimage container, implement `CT_LOCAL_TARBALLS_DIR`; use `__noChroot` as stopgap |
| **TBD-6** | Section 3 | Fetch `.config`, `.config-<crossarch>`, `ctng.patch` via `gh api repos/DataDog/datadog-agent-buildimages/contents/linux/` |
| **TBD-7** | Section 4 | osxcross commit SHA256 + SDK sha256 from `linux/scripts/osxcross.sh` |
| **TBD-8** | Section 4 | MacOSX SDK distribution mechanism — non-redistributable, requires internal cache or `requireFile` |
| **TBD-9** | Section 5 | Guard `packages.dockerImage` behind `lib.optionalAttrs pkgs.stdenv.isLinux` |
| **TBD-10** | Section 8 | Confirm `update-go` task name + `docker-bake.override.json` Go handling retirement |
| **TBD-11** | Section 1b | Extend `tasks/agent.py:104-117` / `dda inv agent.build` to accept `EMBEDDED_PYTHON` as `python_home_3` override when `enable_bazel=False`. Verify exact wiring point in `tasks/agent.py` + `tasks/rtloader.py`. |
