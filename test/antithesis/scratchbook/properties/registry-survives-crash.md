---
slug: registry-survives-crash
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# registry-survives-crash — Auditor Registry Is Readable After Ungraceful Shutdown

## What Led to This Property

SUT guarantee S7: the registry file must be durably and atomically written so
that a crash at any point leaves the registry either fully updated or fully
unchanged (never partially written). The atomic writer (default) achieves this
via `CreateTemp` + write + rename. The non-atomic writer (Fargate, when
`logs_config.atomic_registry_write=false`) does not.

## Code Paths Involved

**Atomic writer** — `comp/logs/auditor/impl/registry_writer.go:23-46`:
1. `os.CreateTemp(registryDirPath, registryTmpFile)` — creates a temp file.
2. `f.Write(data)` — writes JSON.
3. `f.Chmod(0644)`.
4. `f.Close()`.
5. `os.Rename(tmpName, registryPath)` — atomic rename (single filesystem).

Crash safety: at any step before the rename, the original registry file is
intact. After the rename, the new file is complete. The only non-atomic
operation is `Rename` on some filesystems (e.g., NFS, cross-mount).

**Cross-filesystem hazard** (SUT failure mode #8): if `registryDirPath` and
the tmpfile location are on different filesystems, `os.Rename()` fails with
`EXDEV`. The error is logged as `Warn` and the registry is not updated. This
causes silent offset regression: the in-memory registry is correct, but the
on-disk file stays at the previous state indefinitely (until the next successful
flush).

**Non-atomic writer** — `registry_writer.go:56-73`:
1. `os.MkdirAll(filepath.Dir(registryPath), 0755)`.
2. `os.Create(registryPath)` — **truncates the existing file**.
3. `f.Write(data)`.
4. `f.Chmod(0644)`.

Crash between step 2 and 3 leaves a zero-byte file. `recoverRegistry()` reads
the zero-byte file → `json.Unmarshal` fails → returns empty map → all sources
start from default tailing mode.

**Recovery path:**
- `auditor.go:336-354` `recoverRegistry()`: reads the file, unmarshals JSON.
  On any error → empty map → fresh start → loss of all previously tracked
  offsets.

**1-second flush window:**
Even with the atomic writer, the last ≤1 second of in-memory offset updates
are not persisted. A crash in this window → those offsets revert → duplicates
on restart (at-least-once is maintained, data loss is not).

## Failure Scenario

**Atomic writer + EXDEV:**
1. `/var/lib/datadog-agent/run/` is on filesystem A.
2. `os.CreateTemp()` creates the temp file in the same directory — same
   filesystem, so this is safe.
3. But if `registryDirPath` resolves differently than `registryPath`'s directory
   (e.g., symlink to a different mount), the rename fails with EXDEV.
4. Agent logs a warning but continues. The registry on disk drifts behind
   in-memory state.
5. On crash, the stale on-disk registry causes more duplicates than expected.

**Non-atomic writer + crash:**
1. Flush ticker fires.
2. `os.Create()` truncates the file.
3. Antithesis injects a node fault (kill -9).
4. File is zero bytes.
5. On restart: `json.Unmarshal` fails → empty registry → all sources re-tailed
   from end-of-file → permanent loss of in-flight data.

## Why It Matters

Registry corruption is the primary mechanism for large-scale data loss or
duplicate delivery after an ungraceful shutdown. The atomic writer prevents the
worst case (truncation-and-crash), but EXDEV is a silent failure that can
accumulate over time. The non-atomic writer is a known risk on any container
platform where write persistence requires a kernel flush.

Antithesis node-termination faults exercise exactly this scenario. This is
the property most directly testable by kill-based faults.

## Workload Instrumentation

- After each kill-and-restart cycle:
  1. Attempt to parse the on-disk registry file directly (workload has access
     to the agent's volume).
  2. Assert: the file is either (a) the correct JSON from the last successful
     flush, or (b) the correct JSON from the flush before that (within the
     1-second window).
  3. Assert: the file is never zero bytes or unparseable JSON (fails if non-atomic
     writer is tested and crashes mid-write).
- SUT-side: an `Always` assertion at `recoverRegistry()` confirming that the
  recovered registry contains at least as many entries as the agent tracked before
  the crash — currently **missing**.

## Open Questions

- Does the Antithesis environment use a real filesystem with durable rename
  semantics (ext4, xfs) or a container overlay filesystem where rename
  atomicity is not guaranteed? This affects whether the "atomic writer is
  crash-safe" premise holds. `(needs human input)`
- Is there any registry compaction or rotation? The current code uses a single
  file; a corrupt file means total loss. If payload journaling (external ref #2)
  provides an alternative recovery path, the property needs to account for it.
- Is ECS Fargate mode detectable in the Antithesis topology without real AWS
  metadata endpoints? `(needs human input)` — whether the test topology simulates
  Fargate.

## Merged-in evidence (from fargate-registry-no-corruption)

The secondary file provided **ECS Fargate-specific detail** on why
`atomic_registry_write` defaults to false in that environment and what the
fault-trigger requirements are:

**Config default** — `pkg/config/setup/common_settings.go:1987-1988`:
`atomic_registry_write = !IsECSFargate()`. On ECS Fargate the non-atomic writer
is selected by default.

**Fault-trigger specifics** — the node-termination fault must be timed to hit the
non-atomic writer between `os.Create()` (which truncates the file to zero) and
the completion of `f.Write(data)`. This is a narrow but real window: the 1-second
flush ticker fires approximately once per second, and writing a JSON marshal of
potentially hundreds of registry entries requires multiple syscalls.

**Required topology configuration** — the Antithesis topology must configure ECS
Fargate mode (`ECS_FARGATE=true` or `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`)
to exercise the dangerous non-atomic path; otherwise the agent uses the safe
atomic writer and this property is vacuously satisfied.

**Additional assertions needed (from secondary):**
1. `Reachable(non-atomic writer path taken)` — SUT-side in
   `nonAtomicRegistryWriter.WriteRegistry`: confirms the dangerous code path is
   actually exercised, not the safe atomic one.
2. `Sometimes(registry file exists and non-empty after restart)` — confirms at
   least one successful registry write survived a restart during the test run.

### Investigation Log

#### Is `logs_config.atomic_registry_write` default true or false?

- Examined: `pkg/config/setup/common_settings.go:1988`, `pkg/config/env/environment.go:69-71`.
- Found: `config.BindEnvAndSetDefault("logs_config.atomic_registry_write", !pkgconfigenv.IsECSFargate())`. `IsECSFargate()` returns true iff env var `ECS_FARGATE != ""` or `AWS_EXECUTION_ENV == "AWS_ECS_FARGATE"`. So default is **true** (atomic writer) unless one of those env vars is set.
- Found: overrideable via `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false` on any platform — no Fargate detection needed to exercise the non-atomic path.
- Conclusion: resolved. Default is `true` (atomic writer). Non-atomic path requires explicit configuration. Removed from Open Questions; Merged-in evidence section already described this correctly.
