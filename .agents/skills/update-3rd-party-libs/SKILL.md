---
name: update-3rd-party-libs
description: Update a C/C++ third-party library under deps/ — bump version, reconcile sources/copts/config.h/patches, and verify the build.
argument-hint: "<dep-name> [target-version]"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, AskUserQuestion
---

To update a C/C++ third-party library under `deps/`, read **`deps/docs/updating_deps.md`** and follow it end-to-end. That file is the authoritative procedure; its supporting docs (`bump-version.md`, `config-h-refresh.md`, `patches.md`, `verification.md`) live alongside it under `deps/docs/`.

**Arguments:** `$ARGUMENTS` — `<dep-name>` (required), `<target-version>` (optional; ask the user if not supplied). Pass these through to the procedure as its inputs.

The workflow lives under `deps/docs/` (not inline here) so it stays discoverable by humans and non-Claude agents, and is owned alongside the code it touches (`@DataDog/agent-build`).
