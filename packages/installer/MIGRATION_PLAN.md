# Installer: Omnibus → Bazel Migration Plan

## Goal

Produce Bazel `pkg_deb` and `pkg_rpm` targets for Linux whose package contents
match the omnibus-produced packages for `datadog-installer`, then cut over CI
to use the Bazel packages exclusively.

**Status:** Phases 1–4 (BUILD files) complete. Phase 6 (fix differences) partially
complete — `embedded/` dirs and `README.md` done, deb epoch and RPM vendor still
needed. Phase 5 (CI comparison) is next once the remaining fixes land.

---

## Source of Truth

**Omnibus side** (do not modify these during migration):
- `omnibus/config/projects/installer.rb` — package metadata, platform conditions
- `omnibus/config/software/installer.rb` — binary build + file placement; also runs
  the Bazel build inline (see Stage 1 below)
- `omnibus/package-scripts/installer-deb/` — `postinst`, `postrm`
- `omnibus/package-scripts/installer-rpm/` — `posttrans`, `postrm`

---

## How the Current CI Pipeline Works

### Stage 1 — binary + XZ build (`installer-amd64` / `installer-arm64` jobs)

File: `.gitlab/build/package_build/installer.yml`, template `.installer_build_common`

```
dda inv omnibus.build --target-project=installer
```

What omnibus does inside this job:
1. **`preparation.rb`** creates `/opt/datadog-installer/embedded/bin/` and
   `embedded/lib/` with `.gitkeep` placeholders.
2. **`installer.rb` (software)** runs `invoke installer.build --no-cgo …` → binary
   lands at `bin/installer/installer` in the repo root.
3. **`installer.rb` (software)** then runs (via `command_on_repo_root`):
   - `bazelisk build --config=release //packages/installer/linux:whole_distro_tar`
   - `bazelisk build --config=release //packages/installer/linux:debian` (or `:rpm`)
   The Bazel packages are **built here** but are **not uploaded as CI artifacts**.
4. Omnibus license scanner creates `LICENSES/`, `LICENSE`,
   `version-manifest.json`, `version-manifest.txt` inside `install_dir`.
5. **XZ packager** creates `datadog-installer_7-{arch}.tar.xz` containing:
   - `install_dir/*` → everything under `/opt/datadog-installer/`
   - `extra_package_files`: `omnibus/package-scripts/installer-deb/`,
     `omnibus/package-scripts/installer-rpm/`,
     `omnibus/config/templates/installer/README.md.erb`
6. `$OMNIBUS_PACKAGE_DIR/datadog-installer_7-{arch}.tar.xz` is saved as the CI artifact.

### Stage 2 — final deb/rpm packaging

Files: `.gitlab/build/packaging/deb.yml` (`installer_deb-amd64`, `installer_deb-arm64`),
       `.gitlab/build/packaging/rpm.yml` (`installer_rpm-amd64`, `installer_rpm-arm64`,
       `installer_suse_rpm-amd64`, `installer_suse_rpm-arm64`)

```
OMNIBUS_PACKAGE_ARTIFACT_DIR=$OMNIBUS_PACKAGE_DIR
dda inv omnibus.build --target-project=installer
```

`OMNIBUS_PACKAGE_ARTIFACT_DIR` triggers the `package-artifact` software path:
1. **`package-artifact.rb`**: `tar xf *.tar.xz -C /` → files land at their
   absolute paths (e.g., `/opt/datadog-installer/bin/installer/installer`).
2. **`package-artifact.rb`** (installer-specific): renders `README.md` from
   `config/templates/installer/README.md.erb` and places it in `install_dir`.
3. **DEB packager**: copies `install_dir` to staging dir, adds `extra_package_files`
   (the script dirs, extracted from the XZ to their original absolute paths), writes
   `DEBIAN/{control,md5sums,conffiles,postinst,postrm}`, runs `dpkg-deb`.
4. **RPM packager**: same flow with an RPM spec file.

---

## What Each Package Contains

### Omnibus `.deb` — data section (what installs on disk)
```
/opt/datadog-installer/bin/installer/installer       ← Go binary
/opt/datadog-installer/embedded/bin/                 ← empty dir (preparation.rb)
/opt/datadog-installer/embedded/lib/                 ← empty dir (preparation.rb)
/opt/datadog-installer/LICENSES/<many files>         ← omnibus license scanner
/opt/datadog-installer/LICENSE                       ← aggregated license file
/opt/datadog-installer/version-manifest.json
/opt/datadog-installer/version-manifest.txt
/opt/datadog-installer/README.md                     ← rendered from .erb template
```

### Omnibus `.deb` — control section
- `Package`: `datadog-installer`
- `Version`: `1:{PACKAGE_VERSION}-1` (epoch 1, version from env var)
- `Maintainer`: `Datadog Packages <package@datadoghq.com>`
- `Recommends`: `datadog-signing-keys (>= 1:1.4.0)`
- `postinst`, `postrm` scripts

### Omnibus `.rpm` — file list
Same as deb data section above; `.gitkeep` placeholder files are excluded (RPM
ignores empty dirs unless explicitly listed).

### Omnibus `.rpm` — spec metadata
- `Vendor`: `Datadog <package@datadoghq.com>`
- `Packager`: `Datadog, Inc <package@datadoghq.com>` (redhat target)
- `Epoch`: 1
- Pre-requires: `coreutils findutils grep glibc-common shadow-utils` (RHEL);
  `coreutils findutils grep glibc shadow` (SUSE)
- `posttrans`, `postun` scripts

### Bazel `.deb` data section (current state)
```
/opt/datadog-installer/bin/installer/installer       ← Go binary
/opt/datadog-installer/embedded/bin/                 ← pkg_mkdirs (mode 0755)
/opt/datadog-installer/embedded/lib/                 ← pkg_mkdirs (mode 0755)
/opt/datadog-installer/LICENSES/<many files>         ← package_licenses aspect
/opt/datadog-installer/README.md                     ← dd_agent_expand_template
```

---

## Known Differences

| Item | Omnibus | Bazel | Status | Action |
|------|---------|-------|--------|--------|
| Package version | `$PACKAGE_VERSION` (real) | `"7"` (hardcoded) | ❌ Open | ABLD-364: wire `{version}` variable |
| deb epoch | `Version: 1:7-1` in control | version `"7"`, no epoch | ❌ Open | Set `version = "1:7"` in `pkg_deb` (no separate epoch attr) |
| RPM epoch | `Epoch: 1` in spec | `epoch = "1"` | ✅ Done | — |
| RPM vendor/packager | set | unset | ❌ Open | Add `vendor` to `pkg_rpm` |
| `embedded/bin/` + `embedded/lib/` | yes (empty dirs) | yes (pkg_mkdirs) | ✅ Done | — |
| `README.md` | rendered from .erb | dd_agent_expand_template | ✅ Done | — |
| `version-manifest.json/txt` | yes | no | ✅ Accepted drop | omnibus-specific artifact |
| `LICENSE` | aggregated text | no | ⚠️ Pending decision | Add or accept drop |
| `LICENSES/` content | omnibus license scanner | Bazel aspect | ⚠️ Needs verification | Spot-check files match |

---

## Comparison Methodology

Run after both packages are available (see CI integration below).

### A. File-tree comparison

```bash
OMNIBUS_DEB="$(ls $OMNIBUS_PACKAGE_DIR/datadog-installer_*_amd64.deb | head -1)"
BAZEL_DEB="$(ls bazel-bin/packages/installer/linux/datadog-installer_*_amd64.deb | head -1)"

echo "=== File tree diff (< omnibus  > bazel) ==="
diff <(dpkg-deb --fsys-tarfile "$OMNIBUS_DEB" | tar t | sort) \
     <(dpkg-deb --fsys-tarfile "$BAZEL_DEB"   | tar t | sort) || true

# RPM
OMNIBUS_RPM="$(ls $OMNIBUS_PACKAGE_DIR/datadog-installer-*x86_64.rpm | head -1)"
BAZEL_RPM="$(ls bazel-bin/packages/installer/linux/datadog-installer-*x86_64.rpm | head -1)"
diff <(rpm -qlp "$OMNIBUS_RPM" | sort) \
     <(rpm -qlp "$BAZEL_RPM"   | sort) || true
```

### B. Package metadata and scripts comparison

```bash
# DEB control section
dpkg-deb -e "$OMNIBUS_DEB" /tmp/omnibus-ctrl/
dpkg-deb -e "$BAZEL_DEB"   /tmp/bazel-ctrl/
diff /tmp/omnibus-ctrl/control  /tmp/bazel-ctrl/control  || true
diff /tmp/omnibus-ctrl/postinst /tmp/bazel-ctrl/postinst || true
diff /tmp/omnibus-ctrl/postrm   /tmp/bazel-ctrl/postrm   || true

# RPM scripts
diff <(rpm -qp --scripts "$OMNIBUS_RPM") \
     <(rpm -qp --scripts "$BAZEL_RPM")   || true
```

---

## CI Integration: Automated Comparison

The Bazel packages are already built during Stage 1 (`installer-amd64` job) but
the output is not saved. To run the comparison in Stage 2, we rebuild the Bazel
package there (fast: the binary is already extracted from the XZ artifact) and
diff the results.

Both `installer_deb-amd64` and `installer_deb-arm64` have a `script` override in
`.gitlab/build/packaging/deb.yml` (and similarly for rpm variants in `rpm.yml`)
that:

1. Copies the already-extracted binary to the workspace root path expected by
   `@installer_binary//:installer`:
   ```bash
   mkdir -p bin/installer
   cp /opt/datadog-installer/bin/installer/installer bin/installer/installer
   ```

2. Builds the Bazel deb (or rpm):
   ```bash
   bazelisk build --config=release //packages/installer/linux:debian
   ```

3. Runs the comparison and writes output to `$OMNIBUS_PACKAGE_DIR/installer-pkg-diff.txt`
   (which is already in the job's artifact paths).

The diff file is saved as a CI artifact and can be downloaded from any pipeline run.

---

## Target Directory Structure

```
packages/installer/
├── BUILD.bazel         ← exports README.md.in for linux/BUILD.bazel
├── MIGRATION_PLAN.md   ← this file
├── README.md.in        ← template expanded by dd_agent_expand_template
├── linux/
│   └── BUILD.bazel     ← pkg_tar, pkg_deb, pkg_rpm (all targets present)
├── macos/
│   └── BUILD.bazel     ← pkg_tar only (PKG not yet supported)
└── windows/
    └── BUILD.bazel     ← pkg_tar only (MSI not yet supported)
```

---

## Migration Phases

### Phase 1 — Declare prebuilt installer binary in MODULE.bazel ✅ DONE

`prebuilt_file` entry for `@installer_binary` is in `MODULE.bazel`.

### Phase 2 — `packages/installer/linux/BUILD.bazel` ✅ DONE

`pkg_tar`, `pkg_deb`, and `pkg_rpm` targets exist.

### Phase 3 — `packages/installer/macos/BUILD.bazel` ✅ DONE

`pkg_tar` target exists. PKG deferred.

### Phase 4 — `packages/installer/windows/BUILD.bazel` ✅ DONE

`pkg_tar` target exists. MSI deferred.

### Phase 5 — Automated comparison in CI ⏳ PENDING

Add comparison step to packaging jobs so every pipeline produces a diff report.
Deferred until Phase 6 fixes are complete.

**Files to modify:**
- `.gitlab/build/packaging/deb.yml` — add comparison to `installer_deb-amd64` and
  `installer_deb-arm64`
- `.gitlab/build/packaging/rpm.yml` — add comparison to `installer_rpm-amd64`,
  `installer_rpm-arm64`, `installer_suse_rpm-amd64`, `installer_suse_rpm-arm64`

See "CI Integration" section above for the exact steps to add.

**Verification:** After merging, open a pipeline and download
`$OMNIBUS_PACKAGE_DIR/installer-pkg-diff.txt` from any installer packaging job.
Confirm the known differences from the table above appear in the diff.

### Phase 6 — Fix differences 🔄 IN PROGRESS

#### ✅ Done

- **`embedded/bin/` + `embedded/lib/`**: Added via `pkg_mkdirs` in `installer_components`,
  mode `0755`. The `pkg_filegroup` prefix `/opt/datadog-installer` resolves the
  relative paths to the correct absolute destinations.

- **`README.md`**: Added via `dd_agent_expand_template` using `packages/installer/README.md.in`
  as the template. The deb and rpm use different `{uninstall_command}` substitutions
  (`apt-get` vs `yum`) and distinct output filenames (`README_deb.md` / `README_rpm.md`)
  to avoid Bazel output name collisions, then `pkg_files` with `renames` normalises
  each to `README.md` at the install prefix.
  - Deb: included in `whole_distro_tar_deb` (a separate tarball from `whole_distro_tar`
    so the XZ intermediate artifact stays README-free).
  - RPM: included directly in `pkg_rpm.srcs`.

- **RPM `epoch = "1"`**: Already present in `pkg_rpm`.

#### ❌ Still needed

1. **Deb epoch**
   `pkg_deb` has no separate `epoch` attribute (confirmed by reading the rules_pkg
   source). The epoch must be embedded in the `version` field using Debian's
   `epoch:version` format. Change:
   ```python
   version = "7",
   ```
   to:
   ```python
   version = "1:7",
   ```
   Without this, `apt` will not correctly resolve upgrades from omnibus-built
   packages (which carry `Version: 1:7-1` in their control file).

2. **RPM vendor/packager** (verify exact values from comparison)
   ```python
   pkg_rpm(
       name = "rpm",
       ...
       vendor = "Datadog <package@datadoghq.com>",
   )
   ```

3. **Wire up real package version** (ABLD-364)
   `package_name_variables` already reads `PACKAGE_VERSION` from the environment.
   Once ABLD-364 lands, change both `version = "7"` fields to use the stamped value.

#### ⚠️ Pending decisions

4. **`LICENSE` aggregated file** — omnibus generates a concatenated license file.
   Decide whether to generate an equivalent from the Bazel graph or accept the drop.

5. **`LICENSES/` content equivalence** — spot-check a few files from both packages
   to confirm the Bazel `package_licenses` aspect picks up the same third-party
   licenses as the omnibus scanner.

### Phase 7 — Cutover ⏳ PENDING

Once the diff output is clean (or remaining diffs are explicitly accepted):

1. Add standalone Bazel packaging jobs to CI — parallel to the existing omnibus
   jobs initially, for validation:
   ```yaml
   installer_deb-amd64-bazel:
     stage: packaging
     needs: ["installer-amd64"]
     script:
       - mkdir -p bin/installer
       - tar -xJOf $OMNIBUS_PACKAGE_DIR/datadog-installer_*-amd64.tar.xz \
           opt/datadog-installer/bin/installer/installer \
           --strip-components=3 -C bin/
       - bazelisk build --config=release //packages/installer/linux:debian
       - cp bazel-bin/packages/installer/linux/datadog-installer_*.deb $OMNIBUS_PACKAGE_DIR/
   ```

2. Add Bazel installer packages to the size quality gate
   (`tasks/quality_gates/...`).

3. Validate the Bazel-built deb installs correctly on a test Ubuntu/Debian system:
   ```bash
   dpkg -i datadog-installer_*.deb
   ls -la /usr/bin/datadog-bootstrap
   /usr/bin/datadog-bootstrap version
   ```

4. Replace the omnibus packaging jobs with Bazel-only jobs in the YAML files.

5. Update `dd-pkg promote` in
   `.gitlab/distribute/trigger_release/installer.yml` to reference the
   Bazel-built package paths.

6. Mark `omnibus/config/projects/installer.rb` and
   `omnibus/config/software/installer.rb` as deprecated (keep until cutover is
   confirmed in production).

7. Remove `command_on_repo_root` Bazel calls from `omnibus/config/software/installer.rb`
   (they are no longer needed once Stage 2 is purely Bazel).

8. Delete omnibus installer files after one release cycle.

---

## Open Questions

| # | Question | Affects |
|---|----------|---------|
| 1 | Windows install root — `/opt/datadog-installer` or `C:/opt/datadog-installer`? | windows/BUILD.bazel |
| 2 | Should the aggregated `LICENSE` file be generated from the Bazel graph? | Phase 6 |
| 3 | How does `dd-pkg promote` locate packages — by glob or explicit path? | Phase 7 |

---

## Not In Scope (for this milestone)

- Building the installer binary inside the Bazel graph (`cmd/installer/BUILD.bazel`)
  — the `prebuilt_file` approach is sufficient for the packaging migration
- Windows MSI (no Bazel rule exists yet)
- macOS PKG (no Bazel rule exists yet)
- Symbol stripping / debug package split
- Code signing (Windows `sign_file`, macOS `code_signing_identity`)
- Removing omnibus installer code (happens after Phase 7 completes)
