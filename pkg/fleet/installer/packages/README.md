# Package installation hooks

All the packaging tools we use (`dpkg`, `rpm`, the `installer`) allow package-specific code to execute at various stages of the package lifecycle.

## Optional OCI package-owned hooks

An OCI package can include a real `hooks/` directory at the root of its main
package layer. This is a **directory-level opt-in**:

- Without `hooks/`, the installer uses its existing compiled recipes, including
  its existing choice of the Agent's bundled installer for legacy hooks.
- With `hooks/`, the package owns all of the events below. A missing event is a
  no-op, including an entirely empty directory. Compiled recipes are never used
  as a per-event fallback.
- An invalid layout, failing hook, cancellation or timeout fails the operation;
  none of these conditions triggers legacy fallback.

The current installer makes this decision before delegating to a bundled
installer. Thus a package may opt in even when its bundled installer predates
this feature. Old installers ignore the directory and keep their compiled
recipes. DEB/RPM/MSI direct hook dispatch is unchanged; this opt-in is for OCI.
The hidden `hooks` CLI command remains the internal compiled-recipe protocol,
not the entry point for testing package-owned hooks. Use the install/remove and
experiment/extension commands to exercise the full lifecycle.

### Legacy Linux injector recipe retirement

The Agent-owned Linux APM injector recipe (`apm_inject_linux.go` and `apminject/`)
is deprecated in favor of the package-owned hooks shipped by `auto_inject`.
New injector lifecycle behavior belongs in those hooks. The legacy implementation
remains supported for compatibility; security and compatibility fixes may still
need to be backported while it is in use.

Retirement is gradual, with no removal release set. Do not remove the recipe
until supported hookless injector packages, including upgrade/downgrade and
removal paths, and the legacy APM commands, helper scripts and systemd units no
longer require it. An absent `hooks/` directory must continue to use the legacy
recipe during this transition. This deprecation does not apply to other packages'
compiled recipes or Windows injector behavior.

### Executables and input

On Linux/macOS, use an executable regular file with the exact event name, either
a native binary or a script with a shebang. On Windows, resolution order is
`<event>.exe`, `<event>.ps1`, then `<event>.bat`; PowerShell and batch files run
with explicit system interpreters. Batch hook paths containing `%` are rejected
because `cmd.exe` expands environment variables even inside quotes. The hooks
directory and hook files may not be symlinks, directories in place of executables,
or other special files. The normal repository `stable` and `experiment` symlinks
are supported.

The hook runs with the installer's privileges, environment and package root as
its working directory. Its stdin contains one JSON object: the actual existing
`HookContext`, with exactly these fields today:

```json
{
  "package": "datadog-apm-inject",
  "package_type": "oci",
  "package_path": "/opt/datadog-packages/datadog-apm-inject/stable",
  "hook": "postInstall",
  "upgrade": false,
  "windows_args": null,
  "extension": ""
}
```

There are no separate version or OS fields in the current context. Hooks must
ignore future unknown JSON fields and be idempotent: retries and ephemeral
environments may execute them repeatedly. Do not write credentials to stderr.
Windows hooks must not launch nested MSI installations from an active MSI
transaction.

### Events and package-path lifetime

| Event | Package whose root is passed |
| --- | --- |
| `preInstall` | Incoming version in a temporary staging directory, before repository replacement. Do not persist this temporary path. |
| `postInstall` | Newly installed stable version, before the installed-package DB commit. |
| `preRemove` | Current stable version, before deletion; `upgrade=true` for replacement by regular install. |
| `preStartExperiment` | Current stable version. |
| `postStartExperiment` | Newly staged experiment version. |
| `preStopExperiment` | Experiment version, still present on disk for package-owned hooks. |
| `postStopExperiment` | Original stable version after removing the experiment. |
| `prePromoteExperiment` | Original stable version before promotion. |
| `postPromoteExperiment` | Promoted version (the experiment path now also points at stable). |
| `postStartConfigExperiment`, `preStopConfigExperiment`, `postPromoteConfigExperiment`, `resumeConfigExperiment` | Stable version. Recovery is dispatched when the installer daemon resumes configuration experiments. |
| `preInstallExtension`, `postInstallExtension`, `preRemoveExtension` | Base package root, not the extension's layer directory. `extension` identifies the extension. Post-install uses the experiment root when installing for an experiment. |

The `preInstall` lookup needs the incoming layer, so the installer stages that
layer before running pre-installation recipes. Hookless packages retain the
same compiled recipes, hook order and stable-path context. Config extraction and
repository replacement still follow pre-installation. The Linux legacy Agent
experiment-stop delete-first special case is preserved for hookless packages.

No `postRemove` event exists in the current dispatcher. Adding it is outside this
change: a removed package's executable would otherwise no longer exist. Do all
package removal work in `preRemove` (or the appropriate experiment event).

An opted-in package must implement any required service, configuration and
extension cleanup itself; omitting an event does not inherit that work from the
compiled recipe. Extension layer-local hook directories are not dispatched.
For opted-in Windows .NET versions this also means the legacy asynchronous
`UninstallVersion` garbage-collection callback is skipped. Their lifecycle hooks
must clean external per-version dependencies and ensure deletion is safe before
removal, experiment stop or promotion. Hookless versions keep the existing async
callback unchanged; malformed hooks layouts block that cleanup. No new async
hook event is introduced.

### Failure, limits and telemetry

A hook has a five-minute default timeout. Set
`DD_INSTALLER_PACKAGE_HOOK_TIMEOUT` to a positive Go duration (for example `30s`
or `10m`), at most `1h`. Zero, negative, invalid and over-limit durations are
errors when executing a hook; the timeout cannot be disabled. Parent
cancellation also stops execution. Unix hooks run in a separate process group
which is killed on cancellation; Windows cancellation requests tree termination
using the system `taskkill` with a bounded wait. Waiting for inherited output
pipes is bounded as well. Hooks should not daemonize children that evade this
lifecycle.

Stdout is discarded. Stderr capture is limited to 16 KiB, scrubbed using the
installer's standard credential scrubber, and included in failure diagnostics
and the existing telemetry span. Spans record duration, exit code, package hook
source, omitted events, truncation and timeout. Neither stdin context nor the
environment is copied into new telemetry tags.

Hook failure is returned through the existing installer transaction boundary.
There is no new rollback algorithm: Fleet/extension rollback and repository
cleanup retain their existing behavior. For regular install, `postInstall`
already runs after repository replacement; its failure prevents the package DB
commit but can leave the failed payload on disk. Package hooks are responsible
for their own partial side effects, just as compiled hooks were.

This feature executes trusted package code; it is not a sandbox and does not
introduce or claim new OCI signature validation.

## Regular installation / removal / upgrade

The following is valid for `deb`, `rpm`, and `oci` packages.

### Installation

![Install Hooks](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/bfea4958a1e2fddfeac4649c55975d083d6693fd/install.svg)
[Source](https://docs.google.com/drawings/d/1wsqV_Id_utKt7VrT8DAAsP28edk4diBRae59TqnLOAk/edit)

1. For OCI, v1's main layer is staged so package-owned hooks can be discovered.
2. v1's `PreInstall` hook is executed (compiled recipes retain their existing context).
3. v1's files are placed in the repository.
4. v1's `PostInstall` hook is executed.

### Removal

![Remove Hooks](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/f6320aabc00a7a442da3dec11cab6fb83723b7bc/remove.svg)
[Source](https://docs.google.com/drawings/d/1FTWx4drnA_iQTCMUpxzVh-VWLTOgST552eRMkvrikGk/edit)

1. v1's `PreRemove` hook is executed.
2. v1's files are removed from disk.

### Upgrade

![Upgrade Hooks](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/440e3cbc04d9762ee0f1864333ec1a004ec50159/upgrade.svg)
[Source](https://docs.google.com/drawings/d/17RHy35YWuriaeCXTQ5eQciC3goYgle2_Qwe2nhRzzho/edit)

1. v1's `PreRemove` hook is executed. Note that we inform the hook that the package is being upgraded.
2. v2's `PreInstall` hook is executed (its OCI main layer is already staged).
3. v2's files are written to disk and v1's files are removed.
4. v2's `PostInstall` hook is executed. Note that we inform the hook that the package is being upgraded.

## Experiment upgrades

The following is only valid for `oci` packages.

The installer supports a safer upgrade path for `oci` packages called "experiments".

### Package upgrade

![Package Upgrade](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/bfea4958a1e2fddfeac4649c55975d083d6693fd/experiment_package.svg)
[Source](https://docs.google.com/drawings/d/1j2k2vHQhBevPQxJLDAkC68RyMrKysBbJ1Z2ENrYNUxI/edit)

### Start experiment

1. v1's `PreStartExperiment` hook is executed.
2. v2's files are written to disk. v1's files are kept intact.
3. v2's `PostStartExperiment` hook is executed.

### Stop experiment

1. v2's `PreStopExperiment` hook is executed.
2. v2's files are removed from disk. v1's files are kept intact.
3. v1's `PostStopExperiment` hook is executed.

### Promote experiment

1. v1's `PrePromoteExperiment` hook is executed.
2. v1's files are removed from disk. v2's files are kept intact.
3. v2's `PostPromoteExperiment` hook is executed.

# Extensions installation hooks


## Regular installation / removal / upgrade

The following is valid for `deb`, `rpm`, and `oci` packages.

### Installation
When installing package's (v1) extension:

1. v1.extension's PreInstallExtension hook is executed.
2. v1.extension's files are written to disk.
3. v1.extesnion's PostInstallExtension hook is executed.

### Removal
When removing package's (v1) extension:

v1.extension's PreRemoveExtension hook is executed.
v1.extension's files are removed from disk.

### Upgrade

There is no concept of upgrade for an extension. When the package the extension is attached to gets upgraded, its pre-remove script should include removal & save of the installed extensions and its post-install script should include reinstallation of the saved extensions.
