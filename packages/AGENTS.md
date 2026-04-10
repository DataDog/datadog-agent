# packages/

## Summary

This directory contains Bazel package definitions for every distributable artifact
the Datadog Agent project ships. It is the **target state** of the migration away
from Omnibus. See `omnibus/AGENTS.md` for the current source of truth and the
concept mapping between the two systems.

## Directory Tree

```
packages/
├── agent/               ← Main agent (replaces omnibus/config/projects/agent.rb)
│   ├── BUILD.bazel      ← Flavor flags: base, fips, heroku, iot + config_setting groups
│   ├── dependencies/    ← Third-party runtime deps (replaces datadog-agent-dependencies.rb)
│   ├── heroku/          ← Heroku-specific files
│   ├── linux/           ← Linux deb + rpm package targets (most complete example)
│   └── product/         ← First-party binaries and config (replaces datadog-agent.rb)
├── ddot/                ← DDot/OTel (replaces omnibus/config/projects/ddot.rb)
│   ├── BUILD.bazel      ← deb + rpm targets (skeleton — components TODO)
│   ├── debian/
│   └── redhat/
├── dogstatsd/           ← DogStatsD (replaces omnibus/config/projects/dogstatsd.rb)
│   └── BUILD.bazel      ← deb + rpm targets (skeleton — components TODO)
├── install_dir/         ← Shared embedded runtime (Python, libraries)
│   ├── BUILD.bazel      ← :embedded filegroup, :etc filegroup, :install pkg_install
│   └── embedded/        ← Python3 + C libs that live in /opt/datadog-agent/embedded/
├── iot/                 ← IoT Agent (replaces omnibus/config/projects/iot-agent.rb)
│   └── BUILD.bazel      ← deb + rpm targets (skeleton — components TODO)
├── macos/               ← macOS .app bundle components
│   └── app/
├── python3/             ← Python packaging helpers
├── debian/              ← Shared Debian packaging assets (/etc layout)
├── redhat/              ← Shared RPM packaging assets (/etc layout)
├── windows/             ← Windows-specific packaging components
├── rules/               ← Starlark macros shared across package targets
│   └── BUILD.bazel      ← package_naming.bzl — package_name_variables macro
└── todo/                ← Placeholder notes for things not yet started
```

## Key Bazel Patterns

### Naming convention
Every leaf directory that contributes files to a package exposes an `all_files`
target (`pkg_filegroup` or `pkg_files`). Top-level package targets aggregate these:

```
//packages/agent/linux:agent_components   ← everything in /opt/datadog-agent
//packages/agent/linux:transitive_deps    ← C libs picked up transitively (temporary)
//packages/agent/linux:everything         ← components + transitive_deps
//packages/agent/linux:license_files      ← gathered via package_licenses aspect
//packages/agent/linux:whole_distro       ← everything + licenses + build-time items
//packages/agent/linux:whole_distro_tar   ← XZ-compressed tar of whole_distro
//packages/agent/linux:debian             ← .deb artifact
//packages/agent/linux:rpm                ← .rpm artifact
```

### Flavor / variant selection
The `//packages/agent:flavor` string flag controls which variant is built.
Use it (don't use environment variables) for conditional content:

```python
select({
    "//packages/agent:linux_default": [...],   # base flavor on Linux
    "//packages/agent:linux_fips":    [...],   # fips flavor on Linux
    "//packages/agent:linux_heroku":  [...],   # heroku flavor on Linux
    "//conditions:default":           [...],
})
```

### Package scripts
Pre/post install scripts live in `omnibus/package-scripts/` and are referenced
from Bazel by label, e.g.:

```python
preinst = "//omnibus:package-scripts/agent-deb/preinst",
```

They are not being rewritten — only the references move into Bazel.

### License gathering
`package_licenses(name = "license_files", src = ":everything")` walks the
dependency graph via an aspect and collects all third-party license files.
Always include a `:license_files` step before assembling `:whole_distro`.

### Compression
Use `get_compression_level()` from `//bazel/rules:compression.bzl` rather than
hardcoding; it reads the pipeline-injected compression level when available.

## Migration Status

| Product | Omnibus file | Bazel file | Status |
|---|---|---|---|
| Main agent (Linux) | `omnibus/config/projects/agent.rb` | `//packages/agent/linux` | Mostly complete |
| Agent product bundle | `omnibus/config/software/datadog-agent.rb` | `//packages/agent/product` | In progress — many TODO comments |
| Agent dependencies | `omnibus/config/software/datadog-agent-dependencies.rb` | `//packages/agent/dependencies` | In progress |
| DogStatsD | `omnibus/config/projects/dogstatsd.rb` | `//packages/dogstatsd` | Skeleton only |
| IoT Agent | `omnibus/config/projects/iot-agent.rb` | `//packages/iot` | Skeleton only |
| DDot / OTel | `omnibus/config/projects/ddot.rb` | `//packages/ddot` | Skeleton only |
| Installer | `omnibus/config/projects/installer.rb` | *(not started)* | Not started |
| Agent binaries | `omnibus/config/projects/agent-binaries.rb` | *(not started)* | Not started |
| Embedded runtime | `omnibus/config/software/python3.rb` + others | `//packages/install_dir/embedded` | In progress |

## Migration Playbook

To migrate an omnibus project to Bazel, work through these steps in order.
Use `packages/agent/linux/BUILD.bazel` as the worked example throughout.

### Step 1 — Create the package directory

```
packages/<product>/BUILD.bazel
```

Copy the skeleton from an existing file (e.g. `packages/dogstatsd/BUILD.bazel`).

### Step 2 — Map package metadata

| Omnibus | Bazel |
|---|---|
| `name`, `package_name` | `package` attr on `pkg_deb` / `pkg_rpm` |
| `description` | `description` attr (use `_DESCRIPTION` constant) |
| `homepage` | `homepage` attr |
| `license` | `license` attr |
| `maintainer` | `maintainer` attr |
| `build_version` | `version` attr — stamp from pipeline *(TODO for all products)* |
| `build_iteration 1` | `-1` suffix in `package_file_name` |
| `epoch 1` | `epoch = "1"` in `pkg_rpm`, implied in deb filename `1:` prefix |

### Step 3 — Map runtime dependencies and conflicts

| Omnibus | Bazel |
|---|---|
| `runtime_script_dependency :pre, "foo"` | `requires_contextual = {"pre": ["foo"]}` in `pkg_rpm` |
| `runtime_recommended_dependency 'foo'` | `recommends = ["foo"]` in `pkg_deb` |
| `conflict 'foo'` | `conflicts = ["foo"]` in both `pkg_deb` and `pkg_rpm` |

### Step 4 — Map build dependencies to `pkg_filegroup` sources

Each omnibus `dependency 'foo'` becomes a `srcs` entry:

```python
pkg_filegroup(
    name = "product_components",
    srcs = [
        "//packages/<product>/dependencies:all_files",
        "//packages/<product>/product:all_files",
        "//packages/install_dir:embedded",
    ],
    prefix = "/opt/datadog-<product>",
)
```

For each software recipe that does not yet have a Bazel equivalent, add a
`# TODO: dependency 'foo'` comment and a tracking ticket.

### Step 5 — Map extra package files

`extra_package_file '/some/path'` becomes a `pkg_filegroup` entry that places
files at the correct absolute path. Files outside `install_dir` require explicit
path prefixes — do **not** use the `prefix` on the top-level `agent_components`
group for these.

### Step 6 — Map platform conditions

| Omnibus | Bazel |
|---|---|
| `linux_target?` | `select({"@platforms//os:linux": ...})` |
| `windows_target?` | `select({"@platforms//os:windows": ...})` |
| `osx_target?` | `select({"@platforms//os:macos": ...})` |
| `debian_target?` | *(no constraint yet — use `# TODO: select()` comment)* |
| `redhat_target?` | *(no constraint yet — use `# TODO: select()` comment)* |
| `fips_mode?` | `select({"//packages/agent:fips_flavor": ...})` |
| `heroku_target?` | `select({"//packages/agent:linux_heroku": ...})` |

### Step 7 — Map package scripts

Point `preinst`/`postinst`/`prerm`/`postrm` (deb) and
`pre_scriptlet_file`/`post_scriptlet_file`/`preun_scriptlet_file`/
`posttrans_scriptlet_file` (rpm) at the existing scripts in `omnibus/package-scripts/`.

### Step 8 — Wire the license aspect

```python
package_licenses(name = "license_files", src = ":everything")
```

### Step 9 — Assemble the distribution

```python
pkg_filegroup(name = "whole_distro", srcs = [":everything", ":license_files"])
pkg_tar(name = "whole_distro_tar", srcs = [":whole_distro"], ...)
pkg_deb(name = "debian", data = ":whole_distro_tar", ...)
pkg_rpm(name = "rpm", srcs = [":whole_distro"], ...)
```

### Step 10 — Add a `pkg_install` bridge for the omnibus transition period

While both systems coexist, add a `pkg_install` target so the omnibus build can
call Bazel to place files into `INSTALL_DIR`:

```python
pkg_install(
    name = "install",
    srcs = [":all_files"],
    destdir_flag = "//:install_dir",
)
```

This target is removed once the omnibus recipe is deleted.

## Known TODOs Across All Products

- **Version stamping**: all products currently hardcode `version = "7"`. Pipeline
  stamping (equivalent to `build_version ENV['PACKAGE_VERSION']`) is tracked in ABLD-364.
- **RHEL vs SUSE constraint**: `requires_contextual` and `conflicts` use RHEL values
  for all RPM builds. A proper `select()` needs a RHEL vs SUSE platform constraint.
- **Symbol stripping / signing**: `strip_build`, `windows_symbol_stripping_file`,
  `inspect_binary`, and `sign_file` are not yet migrated.
- **Transitive deps**: `//packages/agent/linux:transitive_deps` is a temporary
  catch-all. It shrinks as deps grow proper Bazel targets. See ABLD-363.
