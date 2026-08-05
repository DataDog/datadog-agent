---
slug: registry-format-migration-safe
focus: "8 — Lifecycle Transitions"
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# Property: registry-format-migration-safe

## What led to this property

The auditor registry has gone through three on-disk format versions:
- **v0** (`api_v0.go`): only file offsets (int64), no TailingMode or Fingerprint.
  Identifier stored without `file:` prefix.
- **v1** (`api_v1.go`): added `Timestamp` (container offset as string) and
  `LastUpdated`. Offset stored as int64.
- **v2** (`api_v2.go`): dropped `Timestamp`, replaced with generic `Offset`
  (string). Added `TailingMode`, `IngestionTimestamp`, `Fingerprint`.

The `unmarshalRegistry()` dispatcher (auditor.go:441-462) reads the `Version`
field and routes to the appropriate deserializer. Any unrecognized version causes
the `default` branch to fire, returning `errors.New("invalid registry version
number")` — and `recoverRegistry()` treats this as corrupt data, returning an
empty map.

Key migration behaviors:
- v0 → v2: `unmarshalRegistryV0` rewrites identifiers with `file:` prefix, drops
  entries with offset 0. The `file:` prefix is specific to file tailers. Container
  and journald identifiers from v0 are lost.
- v1 → v2: `unmarshalRegistryV1` preserves `LastUpdated` and `Offset`; drops
  entries with neither a valid offset nor a timestamp. `TailingMode`, `Fingerprint`,
  and `IngestionTimestamp` are zeroed (not present in v1 data).
- After migration the in-memory registry has v2 structure; the next flush writes
  a v2 file. There is no in-place migration or atomic version bump.

The migration is **read-only-correct**: migrating from v1 to v2 for file tailers
produces correct offsets. The edge cases are:
1. **Zero `IngestionTimestamp` in migrated entries** — the `updateRegistry`
   timestamp guard (`auditor.go:385`) compares `v.IngestionTimestamp > ingestionTimestamp`.
   A migrated v1 entry has `IngestionTimestamp = 0`, so the *first live payload*
   for that identifier will always beat the guard (0 < anything), updating the
   registry. This is correct. No issue here.
2. **Agent rollback (v2 agent → v1 agent)**: if a v2 registry is read by a v1
   agent, the v1 deserializer will fail on the unknown `Version: 2` value (if the
   old agent uses a similar dispatch), and the v1 agent may start from scratch. The
   current code's `unmarshalRegistry` dispatch is a forward-migration-only design.
3. **Future v3**: adding a new version without updating `unmarshalRegistry` causes
   the `default` branch to trigger, which is a forward-compatibility hole.

## Key code locations

- `comp/logs/auditor/impl/auditor.go:440-462` — `unmarshalRegistry()`: version
  dispatch. Default branch: unknown version → error → empty map.
- `comp/logs/auditor/impl/api_v0.go:27-45` — v0 migration: identifier rewrite.
- `comp/logs/auditor/impl/api_v1.go:27-45` — v1 migration: Offset selection
  from int64 or string timestamp.
- `comp/logs/auditor/impl/api_v2.go:15-27` — v2 deserialization: direct struct
  unmarshal.
- `comp/logs/auditor/impl/auditor.go:29-32` — `registryAPIVersion = 2` (the
  current write version).

## What fault triggers it

**Agent version upgrade/downgrade** — in normal operation, this is a scheduled
maintenance event. Antithesis can simulate it by injecting a config toggle or
by running two agent versions in sequence against the same persistent volume.

**CPU throttling during version migration** could expose a race between reading
the old-format file and writing the new-format file — if the agent crashes mid-
first-flush after loading a v1 registry, the v2 write is interrupted (especially
on the non-atomic Fargate path), potentially leaving a corrupt file.

## Why it matters

Customers upgrade agents in place, sometimes with rollback. A migration that
silently drops registry entries is a silent replay or loss event that operators
cannot diagnose until they see duplicate logs or missing logs in dashboards.

## Assertions needed (all net-new SUT instrumentation)

1. **`Reachable(v1 migration path taken)`** — SUT-side in `unmarshalRegistryV1`:
   confirms at least one v1 registry file was present and migrated during the test.
2. **`Reachable(v0 migration path taken)`** — SUT-side in `unmarshalRegistryV0`:
   same, for v0.
3. **`Always(migrated registry has same or fewer entries than source)`** — workload-
   side: after migration from v1 to v2, the number of registry entries should equal
   the number of entries in the v1 file that had valid offsets. Zero entries post-
   migration with a non-empty source file indicates a migration bug.
4. **`Unreachable(unknown version number causes empty registry)`** — the `default`
   branch of `unmarshalRegistry()` should never fire in a test that only uses
   versions 0-2. An `Unreachable` assertion at the `default` branch body would
   detect unexpected version values.

## Recovery window requirement

No network faults required; this property exercises the filesystem and startup
path. Node-termination during first post-migration flush is the relevant fault for
the Fargate writer race.

## Open questions

- Does the test topology run two different agent versions sequentially against the
  same persistent volume, or only a single version? A v0/v1→v2 migration can only
  be tested if the test pre-seeds a v0 or v1 registry file.
  `(needs human input)` — topology design decision.
- For the rollback case (v2 → v1 agent downgrade), is this in scope? The code
  does not have a downgrade path, so rollback recovery requires manual registry
  deletion. This could be a separate property. `(needs human input)`

### Investigation Log

#### Confirmed: `recoverRegistry()` is the only startup read path; version dispatch is a code-switch not a migration

- Examined: `comp/logs/auditor/impl/auditor.go:440-462` (`unmarshalRegistry`), `api_v0.go`, `api_v1.go`, `api_v2.go`, `auditor.go:123-129` (`Start()`).
- Found: Version dispatch in `unmarshalRegistry()` is a pure in-memory transformation: it reads the file, picks a deserializer based on the `Version` field, and returns a `map[string]*RegistryEntry`. The migrated data is written to disk only on the next `flushRegistry()` call, which writes in v2 format. If the agent crashes between the first post-migration `Start()` and the first flush tick (1 second), the on-disk file is still in the old format — so migration is idempotent at the cost of re-running on every restart until the first flush.
- No bugs found in the migration logic for versions 0, 1, 2.
- Conclusion: no code-answerable open questions discovered beyond the existing ones. The two remaining questions are topology/design decisions.
