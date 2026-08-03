---
paths:
  - "tools/windows/DatadogAgentInstaller/**"
---

# Windows MSI — prefer the Go fleet installer before editing the MSI

`tools/windows/DatadogAgentInstaller/` contains the Windows MSI code (WiX, custom
actions, properties). Editing it is more complex than it looks: custom actions,
properties, sequencing, and rollback all have to be managed by hand and are hard
to test.

## Rule: justify MSI changes before making them

**Before changing anything under `tools/windows/DatadogAgentInstaller/`, first
consider whether the change can instead be made in the Go fleet automation
installer at `pkg/fleet/installer`.**

That installer has `postinst` and `prerm` hooks which can perform the large
majority of installation and uninstallation operations. Making the change there
is simpler than editing the MSI and managing custom actions and properties.

The MSI is Windows-specific. The Go fleet installer is cross-platform, so putting
installation logic there consolidates behavior across platforms instead of
duplicating Windows-only logic in the MSI.

Do **not** edit the MSI until you have explicitly justified, in your response,
one of:

1. **Why the change cannot be made in `pkg/fleet/installer`** (e.g. it must run
   before the Go installer exists on disk, or it requires MSI-native behavior
   like file installation/registration, per-machine sequencing, or rollback that
   only the MSI can provide), **or**
2. **Why the change is better made in the MSI** than in the Go installer.

If neither can be justified, make the change in `pkg/fleet/installer` instead.

## What is fine to do in the MSI

Wiring that hands work off to the Go installer is expected and does not need
elaborate justification.

- **Passing flags into existing custom actions.** For example, adding a new
  property to the MSI `InstallOCIPackages` custom action (like
  `DD_OTELCOLLECTOR_ENABLED`) is totally fine: it enables installing DDOT, while
  the actual install operations are done by the Go fleet automation installer in
  `pkg/fleet/installer`. The MSI just passes the flag through; the real work
  lives in Go.
- **Adding a new file or directory under `PROJECTLOCATION`** (the install dir,
  i.e. the agent binaries). These are managed only by the MSI today, so adding
  to them in the MSI is fine.

## What is NOT okay in the MSI

- **New config files under `APPLICATIONDATADIRECTORY`, or new
  `ConfigCustomActions`** (`CustomActions/ConfigCustomActions.cs`). Config
  file/layout setup should be done via `pkg/fleet/installer/setup` instead.
- **New Windows SCM services must be justified.** Prefer `procmgr` — see
  `pkg/fleet/installer/packages/processmanager`. procmgr already handles
  restart/recovery, dependency ordering, run-as account, and supervision, and
  does them better than SCM. If a native SCM entry is genuinely needed, create
  it from a Go install hook (like APM SSI), not the MSI.

## Rule of thumb

- Installation/uninstallation **operations** → `pkg/fleet/installer` (postinst / prerm hooks).
- **Config files / layout** (`APPLICATIONDATADIRECTORY`) → `pkg/fleet/installer/setup`, not `ConfigCustomActions`.
- **Managed processes / services** → `procmgr` (`pkg/fleet/installer/packages/processmanager`), not new SCM services (justify any exception).
- **New installable component (new binary/feature)** → deliver as an **agent extension** (version-locked to the agent, flag-gated; model on DDOT's `ddot` extension) or a **standalone OCI package** (independent version; model on `datadog-apm-inject`).
- **Install-dir binaries/files** (`PROJECTLOCATION`) → MSI is fine.
- MSI **plumbing** to trigger or configure Go-installer operations (properties, passing
  flags into `InstallOCIPackages`, etc.) → MSI is fine.
