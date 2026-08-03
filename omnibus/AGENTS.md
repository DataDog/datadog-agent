# omnibus

## Summary
This tree contains the build specifications for the entire Agent project using the
[Omnibus](https://github.com/chef/omnibus) Ruby-based packaging framework.
We are migrating away from it to use Bazel. The canonical target state is described
in `packages/AGENTS.md`.

Use this tree as a **source of truth** for what we build and ship today, but do
not create new files here. Incrementally migrate things to `BUILD.bazel` files
nearer to the source files, following the pattern established in `packages/agent/`.

## Project Structure

### Core Directories

```
omnibus/
├── config/
│   ├── projects/   ← Top-level product definitions (one file = one distributable package)
│   ├── software/   ← Build recipes for individual components/libraries
│   └── templates/  ← ERB templates (e.g. config files, README)
├── package-scripts/ ← Pre/post install shell scripts for deb and rpm packages
│   ├── agent-deb/  (preinst, postinst, prerm, postrm)
│   ├── agent-rpm/  (preinst, posttrans, prerm)
│   ├── dogstatsd-deb/
│   ├── dogstatsd-rpm/
│   ├── ddot-deb/
│   ├── ddot-rpm/
│   ├── installer-deb/
│   ├── installer-rpm/
│   ├── iot-agent-deb/
│   └── iot-agent-rpm/
├── resources/      ← Static assets (macOS entitlements, etc.)
├── files/          ← Static files bundled into packages
├── lib/            ← Ruby helpers (ostools.rb, project_helpers.rb, custom packagers)
└── scripts/        ← Build orchestration scripts
```

### config/projects/ — Product Definitions

Each `.rb` file defines one top-level distributable and its Bazel counterpart:

| Omnibus project | Bazel target | Status |
|---|---|---|
| `agent.rb` | `//packages/agent/linux:{debian,rpm}` | Mostly migrated |
| `dogstatsd.rb` | `//packages/dogstatsd:{debian,rpm}` | Skeleton only |
| `iot-agent.rb` | `//packages/iot:{debian,rpm}` | Skeleton only |
| `ddot.rb` | `//packages/ddot:{debian,rpm}` | Skeleton only |
| `installer.rb` | *(not yet started)* | Not started |
| `agent-binaries.rb` | *(not yet started)* | Not started |

### config/software/ — Component Recipes

Each `.rb` file fetches and builds one component. Key recipes and their Bazel equivalents:

| Software recipe | Bazel equivalent |
|---|---|
| `datadog-agent.rb` | `//packages/agent/product:all_files` |
| `datadog-agent-dependencies.rb` | `//packages/agent/dependencies:all_files` |
| `datadog-agent-finalize.rb` | `//packages/agent/linux:all_files` (cleanup/symlink logic) |
| `datadog-agent-installer-symlinks.rb` | `//packages/agent/linux:datadog_agent_installer_symlinks` |
| `python3.rb` | `//packages/install_dir/embedded:all_files` (via `@cpython`) |
| `systemd.rb` | `@systemd//:all_files` |
| `installer.rb` | *(pending)* |
| `datadog-dogstatsd.rb` | *(pending — part of `//packages/dogstatsd`)* |
| `datadog-iot-agent.rb` | *(pending — part of `//packages/iot`)* |
| `datadog-otel-agent.rb` | *(pending — part of `//packages/ddot`)* |

## How Omnibus Builds Work

An omnibus build:
1. Reads a **project** file (`config/projects/*.rb`) which lists ordered `dependency` entries.
2. For each dependency, runs the corresponding **software** recipe (`config/software/*.rb`),
   which fetches sources and compiles/installs into `INSTALL_DIR` (`/opt/datadog-agent`).
3. Runs packager (deb/rpm/pkg/msi/xz) to create the final artifact.
4. Runs `extra_package_file` directives to add files installed outside `INSTALL_DIR`.
5. Runs `package_scripts_path` scripts as pre/post install hooks.

The two-phase build (build + repackage) is controlled by `OMNIBUS_PACKAGE_ARTIFACT_DIR`:
when set, omnibus skips building and just repackages a pre-built tarball.

## Omnibus → Bazel Concept Mapping

| Omnibus concept | Bazel equivalent |
|---|---|
| `install_dir` | `//:install_dir` flag + `pkg_install` `destdir_flag` |
| `dependency 'foo'` | `srcs = ["//path/to/foo:all_files"]` in `pkg_filegroup` |
| `extra_package_file` | additional `pkg_filegroup` entries outside `install_dir` |
| `package :deb { ... }` | `pkg_deb(...)` in BUILD.bazel |
| `package :rpm { ... }` | `pkg_rpm(...)` in BUILD.bazel |
| `package :xz { ... }` | `pkg_tar(name = "whole_distro_tar", ...)` |
| `package_scripts_path` | `preinst`/`postinst`/`prerm`/`postrm` attrs on `pkg_deb`/`pkg_rpm` |
| `runtime_script_dependency :pre, "foo"` | `requires_contextual = {"pre": ["foo"]}` in `pkg_rpm` |
| `runtime_recommended_dependency 'foo'` | `recommends = ["foo"]` in `pkg_deb` |
| `conflict 'foo'` | `conflicts = ["foo"]` in `pkg_deb`/`pkg_rpm` |
| `linux_target?` / `windows_target?` | `select({"@platforms//os:linux": ..., "@platforms//os:windows": ...})` |
| `redhat_target?` / `debian_target?` | *(pending — no distro constraint yet; use `# TODO: select()`)* |
| `AGENT_FLAVOR` env var | `//packages/agent:flavor` string flag |
| `fips_mode?` | `//packages/agent:fips_flavor` config setting |
| `heroku_target?` | `//packages/agent:linux_heroku` config setting group |
| `build_version ENV['PACKAGE_VERSION']` | Bazel stamping *(TODO — hardcoded `"7"` for now)* |
| `strip_build` | *(TODO — not yet migrated)* |
| `windows_symbol_stripping_file` | *(TODO — not yet migrated)* |
| `inspect_binary` / `sign_file` | *(TODO — not yet migrated)* |

## Package Scripts

The `package-scripts/` directory is still used by Bazel directly — `pkg_deb` and `pkg_rpm`
targets reference scripts as `//omnibus:package-scripts/<product>-<distro>/preinst` etc.
These scripts are **not** being migrated; they move as-is.

## Notes for Future Sessions

- Do not add new software recipes to `config/software/`; instead add Bazel targets
  under `deps/` or the relevant source directory.
- Do not add new project files to `config/projects/`; use `packages/` instead.
- The `packages/agent/linux/BUILD.bazel` file contains a working example of the
  migration pattern and should be the primary reference.
- Transitive dependency tracking (`packages/agent/linux:transitive_deps`) is a
  temporary workaround until all deps are expressed in the Bazel graph.
  See ABLD-363.
