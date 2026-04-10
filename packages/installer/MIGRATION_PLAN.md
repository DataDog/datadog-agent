# Installer: Omnibus → Bazel Migration Plan

## Goal

Produce Bazel `pkg_tar` targets for Linux, macOS, and Windows whose file trees
match the omnibus-produced tarballs for `datadog-installer`. Hand-inspect the
package scripts to confirm correct behavior. Once validated, promote the Linux
`pkg_deb` and `pkg_rpm` targets to CI and retire the omnibus installer build.

## Source of Truth

**Omnibus side** (do not modify these during migration):
- `omnibus/config/projects/installer.rb` — package metadata, platform conditions
- `omnibus/config/software/installer.rb` — binary build + file placement
- `omnibus/package-scripts/installer-deb/` — `postinst`, `postrm`
- `omnibus/package-scripts/installer-rpm/` — `posttrans`, `postrm`

---

## What the Omnibus Build Produces

### Install dir: `/opt/datadog-installer/` (Linux/macOS), `C:/opt/datadog-installer/` (Windows)

```
bin/
  installer/
    installer          ← the Go binary, CGO_ENABLED=0 on Linux
embedded/
  bin/                 ← empty dir (omnibus artifact from preparation.rb — ignore in Bazel)
  lib/                 ← empty dir (omnibus artifact from preparation.rb — ignore in Bazel)
```

The `embedded/` empty dirs are created by `preparation.rb` as scaffolding. They
are not needed in the Bazel build.

### Binary build flags baked in at compile time

The omnibus recipe runs:
```
invoke installer.build --no-cgo \
    --run-path=/opt/datadog-packages/run \
    --install-path=/opt/datadog-installer
```

Which translates to these ldflags:
```
-X github.com/DataDog/datadog-agent/pkg/config/setup.InstallPath=/opt/datadog-installer
-X github.com/DataDog/datadog-agent/pkg/config/setup.defaultRunPath=/opt/datadog-packages/run
```
Plus version ldflags (`pkg/version.Commit`, `pkg/version.AgentVersion`, etc.).

### Package scripts

| Platform | Script | Action |
|---|---|---|
| deb | `postinst` | Creates `/usr/bin/datadog-bootstrap` → `$INSTALL_DIR/bin/installer/installer` |
| deb | `postrm` | On remove/purge: runs `datadog-installer purge`, deletes install and package dirs |
| rpm | `posttrans` | Creates `/usr/bin/datadog-bootstrap` symlink |
| rpm | `postrm` (`%postun`) | On remove: same cleanup as deb `postrm` |

No `preinst`/`prerm` scripts exist.

### Package metadata (Linux)

| Field | Value |
|---|---|
| Package name | `datadog-installer` |
| Maintainer (deb) | `Datadog Packages <package@datadoghq.com>` |
| Maintainer (rpm) | `Datadog, Inc <package@datadoghq.com>` |
| License | Apache License Version 2.0 |
| Section (deb) | utils |
| Priority (deb) | extra |
| Category (rpm) | System Environment/Daemons |
| Epoch | 1 |
| Recommends (deb) | `datadog-signing-keys (>= 1:1.4.0)` |
| RPM pre-requires | `coreutils`, `findutils`, `grep`, `glibc-common`, `shadow-utils` (RHEL) |

---

## Target Directory Structure

```
packages/installer/
├── MIGRATION_PLAN.md   ← this file
├── linux/
│   └── BUILD.bazel     ← pkg_tar, pkg_deb, pkg_rpm
├── macos/
│   └── BUILD.bazel     ← pkg_tar only (no DMG rule yet)
└── windows/
    └── BUILD.bazel     ← pkg_tar only (no MSI rule yet)
```

No top-level `packages/installer/BUILD.bazel` is needed — the installer has no
flavors and no shared config_settings. No `product/` or `dependencies/`
subdirectories are needed at this stage; the installer is a single binary with no
third-party runtime libs.

---

## Migration Phases

### Phase 1 — Declare the prebuilt installer binary in MODULE.bazel

The installer binary is built outside the Bazel graph by `dda inv installer.build`
(just as the agent binary is built by `dda inv agent.build`). We use the same
`prebuilt_file` pattern to make it available as a Bazel target.

**File to change**: `MODULE.bazel`

Add after the `agent_binary` declaration:

```python
prebuilt_file(
    name = "installer_binary",
    target_label = "@@//:bin/installer/installer",
    target_name = "installer",
)
```

The `target_label` path matches what `omnibus/config/software/installer.rb` places
there:
- The recipe copies `bin/installer` → `${INSTALL_DIR}/bin/`, so the binary lives at
  `bin/installer/installer` relative to the repo root.
- On Windows the rule automatically falls back to `installer.exe`.

No BUILD.bazel changes are needed; the `prebuilt_file` rule auto-generates the
repository with a `filegroup(name = "installer")` target.

**How to test**: After adding the declaration, run:
```bash
dda inv installer.build --no-cgo \
    --run-path=/opt/datadog-packages/run \
    --install-path=/opt/datadog-installer
# Then confirm Bazel can see the file:
bazel build @installer_binary//:installer
```

---

### Phase 2 — Create `packages/installer/linux/BUILD.bazel`

Wire the binary into a full Linux package definition. The structure mirrors
`packages/agent/linux/BUILD.bazel` but is much simpler (single binary, no
embedded Python, no transitive C deps).

```python
"""Installer package components specific to Linux."""

load("@rules_pkg//pkg:deb.bzl", "pkg_deb")
load(
    "@rules_pkg//pkg:mappings.bzl",
    "pkg_filegroup",
    "pkg_files",
)
load("@rules_pkg//pkg:rpm.bzl", "pkg_rpm")
load("@rules_pkg//pkg:tar.bzl", "pkg_tar")
load("//bazel/rules:compression.bzl", "get_compression_level")
load("//compliance:package_licenses.bzl", "package_licenses")
load("//packages/rules:package_naming.bzl", "package_name_variables")

package(default_visibility = ["//packages:__subpackages__"])

package_name_variables(
    name = "variables",
    product_name = "datadog-installer",
)

# The installer binary, placed at bin/installer/ within the install dir.
pkg_files(
    name = "installer_binary",
    srcs = ["@installer_binary//:installer"],
    prefix = "bin/installer",
)

# Everything that belongs under /opt/datadog-installer/
pkg_filegroup(
    name = "installer_components",
    srcs = [":installer_binary"],
    prefix = "/opt/datadog-installer",
)

# Intermediate node for the license-gathering aspect.
pkg_filegroup(
    name = "everything",
    srcs = [":installer_components"],
)

package_licenses(
    name = "license_files",
    src = ":everything",
)

pkg_filegroup(
    name = "whole_distro",
    srcs = [
        ":everything",
        ":license_files",
    ],
)

# Primary deliverable for this phase.
pkg_tar(
    name = "whole_distro_tar",
    srcs = [":whole_distro"],
    compression_level = get_compression_level(),
    extension = "tar.xz",
    owner = "0.0",
)

_DESCRIPTION = """Datadog Installer
The Datadog Installer is a lightweight process that installs and updates
the Datadog Agent and Tracers.

See http://www.datadoghq.com/ for more information
"""

pkg_deb(
    name = "debian",
    data = ":whole_distro_tar",
    description = _DESCRIPTION,
    homepage = "http://www.datadoghq.com",
    license = "Apache License Version 2.0",
    maintainer = "Datadog Packages <package@datadoghq.com>",
    package = "datadog-installer",
    package_file_name = "datadog-installer_{version}-1_{arch_deb}.deb",
    package_variables = ":variables",
    postinst = "//omnibus:package-scripts/installer-deb/postinst",
    postrm = "//omnibus:package-scripts/installer-deb/postrm",
    priority = "extra",
    recommends = ["datadog-signing-keys (>= 1:1.4.0)"],
    section = "utils",
    version = "7",  # TODO: stamp from pipeline (ABLD-364)
)

pkg_rpm(
    name = "rpm",
    package_name = "datadog-installer",
    srcs = [":whole_distro"],
    description = _DESCRIPTION,
    epoch = "1",
    group = "System Environment/Daemons",
    license = "Apache License Version 2.0",
    package_file_name = "datadog-installer-{version}-1.{arch_rpm}.rpm",
    package_variables = ":variables",
    posttrans_scriptlet_file = "//omnibus:package-scripts/installer-rpm/posttrans",
    postun_scriptlet_file = "//omnibus:package-scripts/installer-rpm/postrm",
    release = "1",
    # TODO: use select() for RHEL vs SUSE when a platform constraint is defined.
    requires_contextual = {
        "pre": [
            "coreutils",
            "findutils",
            "glibc-common",
            "grep",
            "shadow-utils",
        ],
    },
    summary = "Datadog Installer",
    url = "http://www.datadoghq.com",
    version = "7",  # TODO: stamp from pipeline (ABLD-364)
)
```

**Verification**:
```bash
bazel build //packages/installer/linux:whole_distro_tar
tar tf bazel-bin/packages/installer/linux/whole_distro_tar.tar.xz
```

Expected output includes:
```
./opt/datadog-installer/bin/installer/installer
```

---

### Phase 3 — Create `packages/installer/macos/BUILD.bazel`

macOS uses `pkg_tar` only; no DMG rule exists yet. The install path is identical
to Linux.

Note: on macOS the binary is built **with** CGO (unlike Linux `--no-cgo`). The
`prebuilt_file` rule handles the platform difference transparently — the same
`@installer_binary//:installer` label resolves to the locally-built binary.

```python
"""Installer package components specific to macOS."""

load(
    "@rules_pkg//pkg:mappings.bzl",
    "pkg_filegroup",
    "pkg_files",
)
load("@rules_pkg//pkg:tar.bzl", "pkg_tar")
load("//bazel/rules:compression.bzl", "get_compression_level")
load("//compliance:package_licenses.bzl", "package_licenses")

package(default_visibility = ["//packages:__subpackages__"])

pkg_files(
    name = "installer_binary",
    srcs = ["@installer_binary//:installer"],
    prefix = "bin/installer",
)

pkg_filegroup(
    name = "installer_components",
    srcs = [":installer_binary"],
    prefix = "/opt/datadog-installer",
)

pkg_filegroup(
    name = "everything",
    srcs = [":installer_components"],
)

package_licenses(
    name = "license_files",
    src = ":everything",
)

pkg_tar(
    name = "whole_distro_tar",
    srcs = [
        ":everything",
        ":license_files",
    ],
    compression_level = get_compression_level(),
    extension = "tar.xz",
    owner = "0.0",
)

# TODO: DMG target when pkg_dmg rule is available.
```

---

### Phase 4 — Create `packages/installer/windows/BUILD.bazel`

On Windows the binary is placed differently: at the root of the install dir as
`datadog-installer.exe` (not under `bin/installer/`). The `prebuilt_file` rule
already handles the `.exe` extension fallback.

```python
"""Installer package components specific to Windows."""

load(
    "@rules_pkg//pkg:mappings.bzl",
    "pkg_filegroup",
    "pkg_files",
)
load("@rules_pkg//pkg:tar.bzl", "pkg_tar")
load("//bazel/rules:compression.bzl", "get_compression_level")
load("//compliance:package_licenses.bzl", "package_licenses")

package(default_visibility = ["//packages:__subpackages__"])

# On Windows the binary lives at the install dir root as datadog-installer.exe,
# not under bin/installer/ as on Linux/macOS.
pkg_files(
    name = "installer_binary",
    srcs = ["@installer_binary//:installer"],
    # No prefix: lands at /opt/datadog-installer/datadog-installer.exe
)

pkg_filegroup(
    name = "installer_components",
    srcs = [":installer_binary"],
    prefix = "/opt/datadog-installer",
)

pkg_filegroup(
    name = "everything",
    srcs = [":installer_components"],
)

package_licenses(
    name = "license_files",
    src = ":everything",
)

pkg_tar(
    name = "whole_distro_tar",
    srcs = [
        ":everything",
        ":license_files",
    ],
    compression_level = get_compression_level(),
    extension = "tar.xz",
)

# TODO: MSI target when a Bazel MSI rule is available.
# TODO: Symbol stripping equivalent of windows_symbol_stripping_file.
```

---

### Phase 5 — Hand inspection and validation

With all three `pkg_tar` targets building, verify by hand:

1. **Linux tarball contents**:
   ```bash
   bazel build //packages/installer/linux:whole_distro_tar
   tar tf bazel-bin/packages/installer/linux/whole_distro_tar.tar.xz | sort
   ```
   Compare against the omnibus tarball:
   ```bash
   tar tf <omnibus-built.tar.xz> | sort
   ```
   Expected difference: omnibus includes `embedded/bin/` and `embedded/lib/` empty
   dirs and the package-scripts directories; the Bazel tarball does not.

2. **Package scripts correctness** — inspect the generated deb/rpm:
   ```bash
   bazel build //packages/installer/linux:debian
   dpkg --info bazel-bin/packages/installer/linux/datadog-installer_*.deb
   dpkg-deb -e bazel-bin/packages/installer/linux/datadog-installer_*.deb /tmp/control
   diff /tmp/control/postinst omnibus/package-scripts/installer-deb/postinst
   diff /tmp/control/postrm  omnibus/package-scripts/installer-deb/postrm
   ```
   Repeat for RPM:
   ```bash
   bazel build //packages/installer/linux:rpm
   rpm -qp --scripts bazel-bin/packages/installer/linux/datadog-installer-*.rpm
   ```

3. **Binary correctness**:
   ```bash
   file bazel-bin/external/installer_binary/bin/installer/installer
   # Expected: ELF ... statically linked
   strings bazel-bin/external/installer_binary/bin/installer/installer | grep datadog-installer
   # Expected: /opt/datadog-installer in output
   ```

---

### Phase 6 — Cutover (future)

Once hand inspection passes and the packages install correctly on a test system:

1. Update CI to build `//packages/installer/linux:debian` and
   `//packages/installer/linux:rpm` instead of running omnibus for the installer.
2. Add the Bazel installer targets to the size quality gate.
3. Mark `omnibus/config/projects/installer.rb` and
   `omnibus/config/software/installer.rb` as deprecated.
4. Repeat Phases 1–5 for the next omnibus project (likely `dogstatsd` or `iot-agent`
   — they follow the same single-binary pattern).

---

## Open Questions

| # | Question | Affects |
|---|---|---|
| 1 | The `pkg_files` prefix for Windows — is the install root `/opt/datadog-installer` correct for Windows, or should it be `C:/opt/datadog-installer`? | Phase 4 |
| 2 | Version ldflags (`pkg/version.*`) — how are these stamped in existing Bazel `go_binary` targets? The `prebuilt_file` approach sidesteps this, but when we eventually build the binary inside Bazel we'll need it. | Future |
| 3 | Should `embedded/bin/` and `embedded/lib/` be included as empty `pkg_mkdirs` entries to match omnibus exactly? | Phase 5 |
| 4 | macOS: does the installer binary use the same `/opt/datadog-installer` install path? | Phase 3 |

---

## Not In Scope (this migration)

- Building the installer binary inside the Bazel graph (`cmd/installer/BUILD.bazel`)
  — the prebuilt_file approach is sufficient for the packaging migration
- Windows MSI (no Bazel rule exists yet)
- macOS DMG (no Bazel rule exists yet)
- Symbol stripping / debug package split
- Code signing (Windows `sign_file`, macOS `code_signing_identity`)
- Automated tarball comparison test (deferred; hand inspection is the gate for now)
- Removing omnibus installer code (happens after cutover)
