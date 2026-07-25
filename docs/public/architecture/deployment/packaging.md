# Packaging

-----

The Agent ships through five largely independent packaging channels: Linux deb/rpm packages built by Omnibus (with an incremental migration to Bazel underway), a Windows MSI built with WixSharp, a macOS `.pkg` wrapped in a `.dmg`, container images assembled from `Dockerfiles/`, and OCI "fleet" packages installed by the datadog-installer. This page covers what each artifact contains, how it is built, and where files land on disk. Container images have [their own page](container-images.md), and the OCI package channel is covered in [Fleet automation and the installer](fleet.md). For which processes each package starts and how, see [Process supervision](../processes/supervision.md).

## Key packages and files

| Path | Purpose |
|---|---|
| [`omnibus/`](<<<SRC>>>/omnibus) | The legacy (still primary) build system: project definitions, software recipes, package scripts |
| [`omnibus/AGENTS.md`](<<<SRC>>>/omnibus/AGENTS.md) | How Omnibus builds work; Omnibus-to-Bazel concept map |
| [`omnibus/config/projects/agent.rb`](<<<SRC>>>/omnibus/config/projects/agent.rb) | Main Agent package definition (deb, rpm, pkg+dmg, zip, xz) and flavor handling |
| [`omnibus/package-scripts/`](<<<SRC>>>/omnibus/package-scripts) | deb/rpm/dmg maintainer scripts — now thin shims around the installer binary |
| [`packages/`](<<<SRC>>>/packages) | The Bazel packaging tree — target state of the Omnibus migration |
| [`packages/AGENTS.md`](<<<SRC>>>/packages/AGENTS.md) | Bazel packaging conventions, flavor flags, migration playbook |
| [`packages/agent/linux/BUILD.bazel`](<<<SRC>>>/packages/agent/linux/BUILD.bazel) | The most complete Bazel package: deb/rpm targets and the fleet symlink structure |
| [`packages/rules/`](<<<SRC>>>/packages/rules) | Shared Starlark macros (`package_naming.bzl`, `dd_pkg_deb.bzl`) |
| [`tools/windows/DatadogAgentInstaller/`](<<<SRC>>>/tools/windows/DatadogAgentInstaller) | Windows MSI: WixSharp C# projects and custom actions |
| [`chocolatey/`](<<<SRC>>>/chocolatey) | Chocolatey wrapper packages around the MSI (`datadog-agent`, `datadog-fips-agent`) |
| [`packages/macos/app/`](<<<SRC>>>/packages/macos/app) | macOS launchd plist templates and `Agent.app` bundle resources |
| [`packaging/aix/`](<<<SRC>>>/packaging/aix) | Hand-rolled AIX build (not Omnibus, not Bazel) |
| [`pkg/fleet/installer/packages/datadog_agent_linux.go`](<<<SRC>>>/pkg/fleet/installer/packages/datadog_agent_linux.go) | The actual Linux install logic invoked by maintainer scripts |
| [`tasks/msi.py`](<<<SRC>>>/tasks/msi.py) | Invoke task driving the MSI dotnet build |

## The artifact matrix

| Artifact | Built by | Contents | Installs to |
|---|---|---|---|
| `.deb` / `.rpm` | Omnibus (`agent.rb`) / Bazel (`//packages/agent/linux`) | Full agent tree: all binaries, embedded Python, config examples, systemd/sysvinit/upstart material | `/opt/datadog-agent`, config in `/etc/datadog-agent` |
| SUSE `.rpm` | Omnibus (separate repackage) | Same tree, SUSE-specific scriptlets | Same |
| `.msi` | WixSharp (`DatadogAgentInstaller`) | Agent tree, Windows services, custom actions for user/permission setup | `C:\Program Files\Datadog\Datadog Agent`, data in `C:\ProgramData\Datadog` |
| `.pkg` + `.dmg` | Omnibus (`package :pkg`, `compress :dmg`) | Agent tree, launchd plists, `Agent.app` systray bundle | `/opt/datadog-agent` |
| `.tar.xz` | Omnibus (`package :xz`) | The raw install tree — intermediate artifact consumed by repackaging jobs and container image builds | n/a |
| OCI package | Fleet packaging pipeline | Same agent tree as an OCI image layer, plus install hooks | `/opt/datadog-packages/datadog-agent/<version>` (see [fleet](fleet.md)) |
| Container images | `Dockerfiles/` | Unpacked `.tar.xz` plus s6, entrypoints, init scripts | see [Container images](container-images.md) |
| AIX package | `packaging/aix/` shell stages | Reduced agent (no eBPF, own wrappers) | `/opt/datadog-agent` |

The `.tar.xz` is the pivot of the CI pipeline: a single build stage produces it once per architecture, and separate *repackage* jobs (with `OMNIBUS_PACKAGE_ARTIFACT_DIR` set) re-wrap it into deb, rpm, and SUSE rpm per distro without rebuilding anything. When repackaging, Omnibus health checks are skipped — "building the agent package" in those jobs is pure archive manipulation.

## Flavors

A *flavor* is a build-time variant axis, selected by the `AGENT_FLAVOR` environment variable in Omnibus and by the `//packages/agent:flavor` string flag in Bazel (config settings `linux_default`, `linux_fips`, `linux_heroku`):

| Flavor | Package name | Difference |
|---|---|---|
| `base` | `datadog-agent` | The default, everything included |
| `fips` | `datadog-fips-agent` | FIPS-validated crypto module; `fipsinstall.sh` runs at install/build time |
| `heroku` | `datadog-heroku-agent` | Buildpack-oriented; replaces `trace-loader` with a no-op shim |
| iot | `datadog-iot-agent` | Separate Omnibus project ([`iot-agent.rb`](<<<SRC>>>/omnibus/config/projects/iot-agent.rb)): a single lean Go binary, no Python/rtloader; conflicts with `datadog-agent` |
| dogstatsd | `datadog-dogstatsd` | Standalone DogStatsD server ([`dogstatsd.rb`](<<<SRC>>>/omnibus/config/projects/dogstatsd.rb)), own systemd unit and Windows MSI |
| ddot | `datadog-ddot` | The [DDOT collector](../otel/ddot.md) as an *extension* package layered onto an existing agent install ([`ddot.rb`](<<<SRC>>>/omnibus/config/projects/ddot.rb)) |
| installer | `datadog-installer` | Standalone bootstrapper/daemon package ([`installer.rb`](<<<SRC>>>/omnibus/config/projects/installer.rb)) |

The flavor also surfaces at runtime through [`pkg/util/flavor`](<<<SRC>>>/pkg/util/flavor/flavor.go); see [Binaries and flavors](../processes/binaries.md).

## How Omnibus builds work

An Omnibus **project** ([`omnibus/config/projects/agent.rb`](<<<SRC>>>/omnibus/config/projects/agent.rb)) lists ordered `dependency` entries; each names a **software recipe** in [`omnibus/config/software/`](<<<SRC>>>/omnibus/config/software) that compiles or copies files into `INSTALL_DIR` (`/opt/datadog-agent`). The main recipe, [`datadog-agent.rb`](<<<SRC>>>/omnibus/config/software/datadog-agent.rb), invokes the `dda inv` build tasks; others vendor Python, OpenSSL, and third-party tools. Finally, one or more packagers (`package :deb`, `:rpm`, `:pkg`, `:msi`, `:xz`, ...) wrap the tree, attaching the maintainer scripts from [`omnibus/package-scripts/`](<<<SRC>>>/omnibus/package-scripts).

Some shipped binaries are not built from this repository at all: [`datadog-agent-data-plane.rb`](<<<SRC>>>/omnibus/config/software/datadog-agent-data-plane.rb) downloads a prebuilt Rust `agent-data-plane` tarball pinned by an env var, so repo code and the shipped binary can drift.

### The Bazel migration

The [`packages/`](<<<SRC>>>/packages) tree rebuilds the same artifacts with `pkg_deb`/`pkg_rpm` rules. [`//packages/agent/linux`](<<<SRC>>>/packages/agent/linux/BUILD.bazel) (deb and rpm) is mostly complete; dogstatsd, iot, and ddot are skeletons; the installer package is not started. Two important properties of the migration:

1. Maintainer scripts are **not** rewritten — Bazel references the same `omnibus/package-scripts/` files by label, so there is exactly one copy of install-time behavior.
1. The Bazel deb pre-creates the fleet symlink chain (`datadog_agent_installer_symlinks` target): `/opt/datadog-packages/datadog-agent/stable → /opt/datadog-packages/run/datadog-agent/<version> → /opt/datadog-agent`, plus `experiment → stable`. This makes a package-manager install indistinguishable from an OCI install to the [fleet daemon](fleet.md).

## Where install logic actually lives

The deb/rpm maintainer scripts are deliberately frozen shims. The entire [`agent-deb/postinst`](<<<SRC>>>/omnibus/package-scripts/agent-deb/postinst) is: skip everything if `$DOCKER_DD_AGENT` is set (the package is being unpacked inside a container image build), run `fipsinstall.sh` if present, then delegate to `/opt/datadog-agent/embedded/bin/installer postinst datadog-agent deb`.

The real logic is versioned Go code in [`pkg/fleet/installer/packages/datadog_agent_linux.go`](<<<SRC>>>/pkg/fleet/installer/packages/datadog_agent_linux.go) (`installFilesystem` and the hook functions on the `datadogAgentPackage` variable). It:

1. Creates the `dd-agent` user and group.
1. Enforces the ownership matrix: config and most of the tree belong to `dd-agent`, but `system-probe` and `security-agent` binaries stay `root:root`.
1. Creates the `/usr/bin/datadog-agent` and `/usr/bin/datadog-installer` convenience symlinks and the `/opt/datadog-packages` fleet symlinks.
1. Loads the SELinux policy on RHEL-family systems and writes `install_info`/`install.json`.
1. Renders systemd units from templates **embedded in the installer binary** ([`pkg/fleet/installer/packages/embedded/tmpl/`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl); pre-rendered copies are checked in under `gen/debrpm/` and `gen/oci/`, each with an `-exp` experiment twin), then enables and starts them. The resulting unit topology is described in [Process supervision](../processes/supervision.md).

On systems without systemd, the installer falls back to sysvinit or upstart scripts ([`pkg/fleet/installer/packages/service/`](<<<SRC>>>/pkg/fleet/installer/packages/service), templates in [`packages/debian/etc/`](<<<SRC>>>/packages/debian/etc) and [`packages/redhat/etc/`](<<<SRC>>>/packages/redhat/etc)).

/// warning
To change Linux install behavior, edit the Go hooks in `pkg/fleet/installer/packages/`, not `omnibus/package-scripts/` (the scripts say so in a banner comment). Likewise, never edit the rendered units under `embedded/tmpl/gen/` — they are regenerated from the `.tmpl` sources.
///

## Linux on-disk layout

| Path | Contents | Owner |
|---|---|---|
| `/opt/datadog-agent/bin/agent/agent` | The main multi-call binary | `dd-agent` |
| `/opt/datadog-agent/embedded/bin/` | Satellite binaries: `trace-agent`, `process-agent`, `security-agent`, `system-probe`, `installer`, `trace-loader`, `agent-data-plane`, `dd-procmgrd`, `privateactionrunner`, `secret-generic-connector` | mostly `dd-agent`; system-probe/security-agent `root` |
| `/opt/datadog-agent/embedded/` | Bundled CPython, OpenSSL, shared libraries for Python checks | `dd-agent` |
| `/opt/datadog-agent/ext/ddot/` | DDOT extension payload (`otel-agent`) when the `datadog-ddot` package is installed | `dd-agent` |
| `/etc/datadog-agent/` | `datadog.yaml`, `conf.d/`, `checks.d/`, `auth_token`, `install_info` | `dd-agent` |
| `/etc/datadog-agent/managed/` | Fleet-managed configuration layer (`DD_FLEET_POLICIES_DIR`) | `dd-agent` |
| `/var/log/datadog/` | Agent logs | `dd-agent` |
| `/opt/datadog-packages/` | Fleet package repository and symlinks (see [fleet](fleet.md)) | root |
| `/usr/bin/datadog-agent` | Symlink to the main binary | root |

## Windows MSI

The MSI is built from [`tools/windows/DatadogAgentInstaller/`](<<<SRC>>>/tools/windows/DatadogAgentInstaller): the WiX XML is *generated* by C# code (WixSharp) in the `WixSetup` project (`AgentInstaller.cs` under `WixSetup/Datadog Agent/`). Omnibus only produces the file tree (`package :msi` sets `skip_packager`); [`tasks/msi.py`](<<<SRC>>>/tasks/msi.py) drives the dotnet build that turns the tree into the installer.

Install-time behavior lives in [`CustomActions/`](<<<SRC>>>/tools/windows/DatadogAgentInstaller/CustomActions):

1. User and permission management (`ProcessUserCustomActions.cs`, `ConfigureUserCustomActions.cs`) — the Agent runs as a dedicated Windows user supplied via the `DDAGENTUSER_NAME`/`DDAGENTUSER_PASSWORD` properties.
1. Service registration (`ServiceCustomAction.cs`) — service names, accounts, and SCM dependencies (see [Process supervision](../processes/supervision.md)).
1. Python distribution installation (`PythonDistributionCustomAction.cs`).
1. OCI package installation from *within* the MSI (`InstallOciPackages.cs`) and fleet handoff (`PatchInstallerCustomAction.cs`, `SetupInstallerCustomAction.cs`) — this is how a Fleet-driven Windows upgrade re-enters MSI land.

Other notable MSI properties: `APIKEY`, `PROJECTLOCATION` (Program Files directory), `APPLICATIONDATADIRECTORY` (defaults to `C:\ProgramData\Datadog`, which holds `datadog.yaml`, `conf.d/`, and logs), and `FLEET_INSTALL`. There is a second MSI project (`WixSetup/Datadog Installer`) for the standalone Datadog Installer service. The [Chocolatey packages](<<<SRC>>>/chocolatey) and the standalone dogstatsd MSI ([`omnibus/resources/dogstatsd/msi/`](<<<SRC>>>/omnibus/resources/dogstatsd/msi)) wrap these installers.

## macOS

The same Omnibus project produces a product archive (`package :pkg`, installed to `/opt/datadog-agent`, signed with the Datadog Developer ID when `SIGN_MAC=true`) compressed into a `.dmg`. Maintainer scripts live in [`omnibus/package-scripts/agent-dmg/`](<<<SRC>>>/omnibus/package-scripts/agent-dmg). Service management is launchd: the postinst installs LaunchDaemons rendered from the plist templates in [`packages/macos/app/`](<<<SRC>>>/packages/macos/app) (`com.datadoghq.agent`, `com.datadoghq.sysprobe`, `com.datadoghq.data-plane`) plus per-user LaunchAgents for the systray GUI. There are no separate trace-agent or process-agent services on macOS — see [Process supervision](../processes/supervision.md).

## AIX

AIX is entirely separate from both build systems: [`packaging/aix/`](<<<SRC>>>/packaging/aix) drives numbered shell stages (checkout, Python, rtloader, agent, integrations, assemble) with its own package scripts and service wrappers (`agent-wrapper.sh`, `trace-agent-wrapper.sh`).

## Gotchas

1. **Two-phase builds change what "build" means.** With `OMNIBUS_PACKAGE_ARTIFACT_DIR` set, the deb/rpm jobs only repackage a prebuilt `.xz`; Omnibus health checks are skipped. If you need a real rebuild, run the build stage, not the repackage stage.
1. **`DOCKER_DD_AGENT` doubles as an install-time guard.** The deb postinst exits immediately when it is set, so installing the deb inside a custom container image performs none of the user, symlink, or unit setup — this is intentional (the [container images](container-images.md) provide their own equivalents) but surprises people building custom images.
1. **The installer binary is inside the agent package.** `/opt/datadog-agent/embedded/bin/installer` is a dedicated installer binary built alongside the agent in [`datadog-agent.rb`](<<<SRC>>>/omnibus/config/software/datadog-agent.rb); when it is absent (for example the Heroku flavor), the finalize step symlinks it to the agent multi-call binary, which registers an installer personality under the `bundle_installer` build tag ([`cmd/agent/installer.go`](<<<SRC>>>/cmd/agent/installer.go)). Either way, the postinst of the package literally runs code shipped by the package it is installing, so a corrupted embedded installer breaks package installation itself.
1. **Flavors are separate package names, not sub-options.** `datadog-agent`, `datadog-fips-agent`, `datadog-heroku-agent`, and `datadog-iot-agent` conflict with each other; switching flavor means uninstall and reinstall.
1. **`agent-data-plane` is version-pinned externally.** The Rust ADP binary is downloaded prebuilt at Omnibus build time; do not expect `pkg/` sources and the shipped ADP to match.
