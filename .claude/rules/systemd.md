---
paths:
  - "pkg/fleet/installer/packages/embedded/tmpl/**/*.service.tmpl"
  - "pkg/fleet/installer/packages/embedded/tmpl/gen/sd/**"
  - "pkg/fleet/installer/packages/embedded/tmpl/gen/pm/**"
---

# Linux systemd — prefer procmgr before a new unit

## Rule: justify new systemd units before adding them

**Before adding a new `datadog-agent-*.service` (or expanding a unit to own a
new long-running Agent child), prefer supervising it with procmgr.**

Add a `processes.d` entry via `pkg/fleet/installer/packages/processmanager`
(and the embedded templates). procmgr already handles restart/recovery,
dependency ordering, and gates (`condition_path_exists`,
`condition_config_any` / `condition_config_none`).

Do **not** add a new systemd unit until you have explicitly justified, in your
response, one of:

1. **Why the process cannot run under procmgr** on Linux yet, or
2. **Why a native systemd unit is required** (e.g. it must be the root Agent /
   procmgr daemon itself, or it needs systemd-only semantics that procmgr does
   not provide).

## What is fine as systemd

- The core Agent unit and `datadog-agent-procmgr.service`.
- Existing sub-service units that have not migrated yet (do not grow their
  scope without considering procmgr).
- Unit wiring that only starts/stops or Wants procmgr / the core Agent.

## Rule of thumb

- New long-running Agent child on Linux → `processes.d` + procmgr first.
- New Windows SCM service → same preference (see `.claude/rules/msi.md`).
