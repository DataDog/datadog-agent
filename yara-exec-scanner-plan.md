# YARA Exec Scanner PoC — Work Plan

**Area:** CWS / system-probe · **Status:** M1 + M2 done (local Lima dry run passed, 2026-10-01); M3/M4 not started · **Engine for the PoC:** libyara (see WS-A) · **Platforms:** Linux amd64 / arm64
**Threat model (PoC):** unprivileged attacker on local filesystems

Goal: scan every executed binary with YARA rules, once per distinct file content, without slowing the event path.

---

## 1. Why this design

Hooking YARA into security-profile insertion does not work:

- Activity-tree nodes are matched on path, lineage and args (`ProcessInfo.matches`, `pkg/security/security_profile/activity_tree/process_node.go:339`). The file hash is never used for matching.
- Hashes are computed only when a new node is created (`NewProcessNode`, `process_node.go:194`).
- So a binary replaced in place at a known path, or a new image tag with changed binaries, never creates a new node and would never be scanned.
- V2 profiles (`manager_v2.go:580`) skip host processes (null container context).

Instead the PoC registers an **event-monitor consumer for exec events**:

- `DispatchEvent` calls `sendEventToConsumers` for every event (`pkg/security/probe/probe_ebpf.go:1225`).
- Each consumer's `Copy()` runs on the hot path, then the result goes into a buffered channel with a non-blocking send (`pkg/security/probe/probe.go:349`). When the channel is full, the event is dropped and counted.
- All heavy work (open, hash, scan) runs on the consumer's own goroutines.

Two dedupe levels keep the cost low:

1. **File identity** `(MountID, Inode, CTime)`, taken from the exec event in memory with no syscall. Most execs stop here. An unprivileged user can't set ctime, and any content write changes it.
2. **Content**: an in-memory sha256 set. The same binary across many containers or paths is scanned once.

The file is read once. The same bytes are hashed and scanned, so the file can't change between the two (no TOCTOU).

## 2. Scope

**In the PoC**
- All execs, host and containers, eBPF probe
- The main binary, plus the script file for interpreter execs
- Identity cache and sha256 set, both in memory
- Always re-hash on FUSE / network filesystems
- Periodic re-check TTL on identity hits
- Rules loaded from a local directory at startup
- Output: structured log line and metrics
- Off by default behind a config flag

**Out of the PoC**
- Protection against a root attacker (clock change, raw block-device writes, debugfs)
- `inode->i_version` from eBPF as the identity key
- IMA measurement log
- Scanning opened (non-exec) files
- Rule delivery via Remote Config
- Backend event or signal, and UI
- Storing verdicts in security profiles
- Windows, macOS, ebpfless mode

## 3. Data flow

```mermaid
flowchart LR
  K[eBPF exec event] --> D[DispatchEvent]
  D --> C["Copy(): small ExecFile struct"]
  C -->|non-blocking send| Q[(consumer channel)]
  Q --> H[HandleEvent goroutine]
  H --> I{identity seen?}
  I -->|yes, TTL fresh| X[drop]
  I -->|no / stale / FUSE-NFS| O["open /proc/pid/exe or container root"]
  O --> R[read once, sha256]
  R --> S{sha256 claimed?}
  S -->|yes| X
  S -->|no| W[(scan queue)]
  W --> P[YARA worker pool]
  P --> L[log + metrics]
```

## 4. Shared contracts

Agree on these types first, so WS-B, C, D and F can build against each other using stand-in implementations.

**Landed:** the contracts live in `pkg/eventmonitor/consumers/yara/types.go` (source of truth; the snippet below is a summary). Stand-ins in `standin.go`: `StandInDeduper` (unbounded maps), `MarkerScanner` (matches a marker string), `LogReporter`, and `InlineScanPool` (scans synchronously). Changes to `types.go` must be agreed across workstreams.

In addition to the snippet below, `types.go` defines the hand-off between WS-C and WS-D:

```go
// ScanJob: bytes read once, hashed into Sum, passed as-is to the scanner.
type ScanJob struct {
    File ExecFile
    Sum  [32]byte
    Data []byte
    Done func() // called exactly once when the pool is done with Data (returns the buffer)
}

type ScanPool interface {
    Submit(job ScanJob) bool // non-blocking; on false the pool has released the hash and called Done
}
```

```go
// ExecFile is what Copy() extracts from the event. Keep it small: it is built on the hot path.
type ExecFile struct {
    PID         uint32
    CGroupID    containerutils.CGroupID
    ContainerID containerutils.ContainerID
    Path        string // Process.FileEvent.PathnameStr
    MountID     uint32 // FileEvent.PathKey.MountID
    Inode       uint64 // FileEvent.PathKey.Inode
    CTime       uint64 // FileEvent.FileFields.CTime
    Filesystem  string // for the FUSE/NFS bypass
    IsScript    bool   // true for LinuxBinprm.FileEvent (interpreter execs)
    SeenAt      time.Time
}

type Identity struct {
    MountID uint32
    Inode   uint64
    CTime   uint64
}

// Deduper holds both dedupe levels. Must be safe for concurrent use.
type Deduper interface {
    IdentityFresh(id Identity, now time.Time) bool // true: skip, no I/O
    MarkIdentity(id Identity, now time.Time)
    ClaimHash(sum [32]byte) bool                  // true: caller owns the scan
    ReleaseHash(sum [32]byte)                     // on drop/failure, so a later exec retries
}

// Scanner wraps the YARA engine. Rules are compiled once at startup.
type Scanner interface {
    Scan(ctx context.Context, data []byte) ([]Match, error)
    RulesVersion() string
}

type Match struct {
    Rule      string
    Namespace string
    Tags      []string
}

// Reporter emits results. PoC: log + metrics.
type Reporter interface {
    Report(f ExecFile, sum [32]byte, matches []Match, err error)
}
```

> Note: `FileFields` has no size field (`pkg/security/secl/model/model_unix.go`). The identity key is therefore `(MountID, Inode, CTime)`. The worker gets the size from `fstat` after opening the file.

## 5. Workstreams

### WS-A — YARA engine spike and decision
**Size:** M · **Can start:** now

Pick between yara-x (Rust, C API + Go bindings) and libyara via go-yara. Either one adds cgo to system-probe.

- [x] Hello-world scan with each engine, linked into a Go binary, linux amd64 and arm64 — arm64 only; amd64 not measured
- [x] Measure scan time for 1 MB / 20 MB / 100 MB binaries against a realistic ruleset (~500 rules)
- [x] Measure memory used by compiled rules, and the binary size delta (the size quality gate checks it) — +0.38 MiB stripped for the final libyara build (M2)
- [x] Check static linking, glibc floor, license (both BSD-3) and CVE history — glibc floor on the agent's build sysroot still open (WS-E)
- [ ] With the build team, check that the native lib can be built under Bazel (and omnibus if needed) — desk research only (rules_rust, deps/ patterns); build-team review still to do

**Done when:** a short decision doc with numbers is agreed, plus a throwaway `Scanner` prototype on the chosen engine.

**Outcome (2026-09-30):** spike on branch `yara/ws-a` (`spike/yara-engine/DECISION.md`, arm64 only; amd64 not measured). 503 signature-base rules:

| | libyara 4.5.8 + go-yara | yara-x 1.21 slim |
|---|---|---|
| Scan 1 / 20 / 100 MB | 4.4 / 88 / 450 ms | 2.8 / 43.5 / 249 ms |
| Compiled rules RSS | 11.3 MiB | 19.4 MiB |
| Binary delta (stripped / gzip) | +0.6 / +0.26 MiB | +20.6 / +7.8 MiB |
| Build | ~3 s, C, `cc_library` | ~110 s, Rust, ~196 crates |

The spike recommends yara-x for production (memory-safe, libyara is in maintenance mode, ~1.8× faster, Pulley interpreter avoids JIT memory).

**Decision for the PoC: libyara 4.5.8 + go-yara**, built without crypto and with only the modules we need (elf, pe, hash, math, …). It keeps WS-E small (one `cc_library`, +0.6 MiB, no Rust toolchain or crate tree) and the engine sits behind WS-D's `Compiler` seam, so switching later is one new `Scanner` implementation plus build work. **Switching to yara-x is a gate item for M4** (before any wider pilot), because libyara parses untrusted binaries in C as root.

### WS-B — Exec consumer, config and wiring
**Size:** M · **Can start:** now, with stand-ins

Consumer skeleton: receive exec events, build `ExecFile`, hand it to the dedupe pipeline. Model it on `pkg/eventmonitor/consumers/process.go`.

- [x] Implement `probe.EventConsumerHandler` (`ID`, `ChanSize`, `EventTypes` = exec, `Copy`, `HandleEvent`); interface at `pkg/security/probe/eventconsumer.go`
- [x] Implement `eventmonitor.EventConsumer` (`ID`, `Start`, `Stop`); interface at `pkg/eventmonitor/consumer.go`
- [x] `Copy()`: extract the main binary's fields. When `HasInterpreter()`, also emit the `LinuxBinprm.FileEvent` entry. Skip kworkers. Resolve `Filesystem` in `Copy()` only if it's cheap, otherwise in the worker — fixed after the dry run: paths are resolved lazily, and for #! execs `Process.FileEvent` is the script and `LinuxBinprm` the interpreter
- [x] Register in `cmd/system-probe/modules/eventmonitor_linux.go` (next to `createProcessMonitorConsumer`), gated by config
- [x] Ensure enabling YARA turns on the event monitor module even when CWS rules are off (check the adjust logic in `pkg/system-probe/config`)
- [x] Add config under `event_monitoring_config.yara.*` in `pkg/config/schema/yaml/system-probe_schema.yaml` (use the `/create-config-field` skill)
- [x] Split by build tag: the real consumer behind the `yara` tag, a no-op stub otherwise, so the default build stays cgo-free until WS-E lands — done as an opt-in `yara` tag selecting the libyara engine; the stand-in engine is used without it (M2)

**Done when:** with stand-in Deduper and Scanner implementations, running `/bin/true` on a dev VM logs an `ExecFile` line with correct fields, for both host and container execs.

### WS-C — Dedupe and file access
**Size:** M · **Can start:** now

- [x] Identity LRU (`Identity → lastChecked`), size from config. `IdentityFresh` returns false after `recheck_ttl`
- [x] sha256 set with an in-flight state: `ClaimHash` marks the hash, `ReleaseHash` clears it on queue-full or scan error — bounded LRU (100k); no separate in-flight state needed
- [x] Skip the identity level when `Filesystem` is fuse or a network filesystem (nfs, nfs4, cifs, smb3, 9p)
- [x] Open order: — plus inode+ctime check on the path fallbacks (M1)
  1. `/proc/<pid>/exe` (`utils.ProcExePath`, `pkg/security/utils/proc_linux.go:146`)
  2. `utils.ProcRootFilePath(pid, path)`
  3. The container's other PIDs, from the cgroup resolver

  Copy the loop in `pkg/security/resolvers/hash/resolver_linux.go:329-372`, or extract a shared helper if that's clean.
- [x] For script entries, skip `/proc/pid/exe` (it points at the interpreter)
- [x] Call `fstat`. Skip non-regular files, and skip files over `max_file_size` (counted)
- [x] Read once into a pooled buffer, sha256 those bytes, and pass the **same** buffer to the scanner
- [x] Unit tests: identity hit, ctime change, TTL expiry, FUSE bypass, claim/release, concurrent claims

**Done when:** unit tests pass. Running the same binary 10,000 times causes one read, and rewriting the binary causes exactly one more.

### WS-D — Scanner pool and rule loading
**Size:** M · **Can start:** now with a stand-in scanner; real engine after WS-A

- [x] Bounded queue (`queue_size`) and `workers` goroutines. Submit is non-blocking; when the queue is full, call `ReleaseHash` and count a drop
- [x] Per-scan timeout (`scan_timeout`). On timeout, count it and release the hash — libyara timeouts are whole seconds
- [x] At start, load and compile every rule file in `rules_dir`. On a compile error, log it and disable scanning; never crash system-probe — all-or-nothing: one bad rule disables scanning (see findings); rule files must be root-owned
- [x] `RulesVersion()` returns a hash of the rule file contents; include it in every report
- [x] Stand-in scanner that matches a marker string, for tests and M1 (`MarkerScanner`, landed with the contracts)
- [x] Real engine implementation: libyara via go-yara (WS-A decision), as a `Compiler` (`engine_libyara.go`, M2). Requirements from the spike:
  - compiled rules are shared; each worker needs its **own** scanner object (`yr.NewScanner(rules)` per worker)
  - a scan can't be cancelled once started, and the engine timeout is in **whole seconds**: check `ctx.Err()` before scanning, set the engine timeout to `ceil(time until ctx deadline)`
  - keep `Scan` synchronous, so the buffer is never returned to the pool while C code still reads it
  - compile from `.yar` source only; never load pre-compiled (serialized) rules (GHSA-2jx3-ff3v-j7jj)
  - behind the `yara` build tag (cgo); the default build keeps the stand-in
- [x] Cap memory: buffered bytes ≤ `workers × max_file_size` — byte budget over queued + in-flight jobs

**Done when:** with the stand-in scanner, flooding the queue drops scans cleanly without blocking. With the real engine, a test rule matches a test binary.

### WS-E — Build and packaging
**Size:** L · **Can start:** after WS-A

Likely the longest task. Involve the build and packaging owners early.

- [x] Add the native lib and Go binding as dependencies (Bazel; see `bazel/AGENTS.md`). For libyara: a `cc_library` under `deps/` (see the `update-3rd-party-libs` skill), `--without-crypto`, only the needed modules, static link. go-yara uses `#cgo pkg-config`, which rules_go doesn't support: patch the binding or add a thin cgo wrapper
- [ ] Check the glibc floor on the agent's build sysroot (the spike built on glibc 2.39)
- [x] Add a `yara` build tag to the system-probe flavors in `tasks/build_tags.bzl`, Linux only (opt-in for now: `SYSTEM_PROBE_YARA_TAGS`, `dda inv system-probe.build --yara`)
- [ ] Package with the agent (`packages/`; omnibus only if unavoidable)
- [x] Update `LICENSE-3rdparty.csv` (go-yara; libyara is tracked by its Bazel `license()`, like the other C deps)
- [ ] Run the size quality gate and report the delta
- [x] Confirm `dda inv system-probe.build` works with and without the tag (arm64)

**Done when:** a CI-built system-probe (amd64 and arm64) includes the scanner, and the size delta is known and accepted.

### WS-F — Reporting and metrics
**Size:** S · **Can start:** now

- [x] Structured log on match: path, pid, container ID, sha256, rule names, namespace, tags, rules version
- [x] Counters:
  - execs received
  - channel drops (the probe already exposes `eventDropped`)
  - identity hits, sha hits
  - reads, read errors by reason, too-big
  - scans, matches, scan errors, timeouts, queue drops
- [x] Gauges: identity cache size, sha set size, queue depth. Distribution: scan duration
- [ ] Stretch goal, scoped as a separate follow-up: send a custom event to the CWS backend (the consumer needs a way to reach the CWS event sender) — not attempted

**Done when:** metrics are visible in a dev-org dashboard during the M1 dry run.

### WS-G — Tests and performance
**Size:** M · **Can start:** after WS-B, C, D

- [ ] Integration test using `pkg/eventmonitor/consumers/testutil`: exec a test binary that contains a marker string, and assert exactly one match report — written (`pipeline_evm_test.go`) but skips without eBPF; not in CI
- [ ] Scenarios: — all checked by hand in the Lima dry run, not automated
  - same binary run many times → one scan
  - binary rewritten in place → re-scan
  - same binary copied to two paths → one scan
  - interpreter exec → the script file is scanned
  - container exec
  - deleted binary still running
- [ ] Exec-storm benchmark (a tight `/bin/true` loop plus a realistic mix): CPU, RSS, channel drops and event latency, feature on vs off — rough numbers only from the dry run (~97% of execs skipped without a read, scans 0.02–38 ms); no on/off comparison
- [ ] Cold-start test: many distinct binaries at once (image pull, then start); measure time to drain the queue
- [ ] Tune the defaults for `chan_size`, `workers`, `queue_size` and `max_file_size`

**Done when:** tests run in CI, and a benchmark report with the steady-state overhead per exec is attached.

## 6. Dependencies

| Workstream | Size | Depends on | Can start |
|---|---|---|---|
| WS-A Engine spike | M | — | now |
| WS-B Consumer + config | M | shared contracts | now, with stand-ins |
| WS-C Dedupe + file access | M | shared contracts | now |
| WS-D Scanner pool | M | WS-A (real engine) | now, stand-in scanner |
| WS-E Build + packaging | L | WS-A | after decision |
| WS-F Reporting | S | shared contracts | now |
| WS-G Tests + perf | M | B, C, D (E for CI) | after M1 |

Sizes are relative: S ≈ a few days, M ≈ 1–2 weeks, L ≈ several weeks (mostly waiting on build infrastructure).

## 7. Milestones

1. ✅ **M1: dry run with no cgo.** WS-B + WS-C + WS-D with the stand-in scanner + WS-F. Logs `would scan sha256=…` and emits metrics on a dev VM. Shows the dedupe ratio and overhead before any native code.
2. ✅ **M2: real engine in a local build.** WS-A decided and WS-D has the real engine. A test rule matches a test binary on a locally built system-probe.
3. ⏳ **M3: CI build and tests.** WS-E lands; WS-G integration tests run in CI on amd64 and arm64.
4. ⏳ **M4: performance validated.** WS-G benchmark report, defaults tuned, go/no-go for a wider pilot. **Gate:** engine switched to yara-x (or an explicit, reviewed exception to stay on libyara).

**Status (2026-10-01):** M1 was folded into M2: the pipeline was wired with the stand-in engine, then validated directly with the real libyara engine on a local Lima VM (Ubuntu 24.04 arm64, real eBPF, system-probe standalone). Passed: marked ELF (1 scan, then identity hits), `#!` script, same content at a 2nd path (sha hit), rewrite in place (rescan), 2,000-exec storm (no rescans), exec in a container (match with container ID); ~97% of execs skipped without any read. M3 is blocked on push access to the repo; M4 not started.

### Dry-run findings (to address before a pilot)

- [ ] **Known-bad content isn't re-reported.** The sha256 set remembers "scanned", not the verdict, so the same malware under a new path, container or identity runs without an alert. Cache the matches per sha256 and report again on a hit, without rescanning.
- [ ] **Exec paths need CWS enabled.** With `runtime_security_config.enabled: false`, exec paths aren't resolved (empty `PathnameStr`), so only `/proc/<pid>/exe` is usable: short-lived binaries and all scripts are missed. Either require CWS or find the switch that enables path resolution in event-monitor-only mode.
- [ ] **Rule compatibility.** One failing rule file disables all scanning; external variables (`filename`, `filepath`, `extension`, `owner`, …) aren't defined; the `hash` module needs crypto (we build without). Compile files independently and skip failures, define the common externals, consider linking the agent's OpenSSL for `hash`. `pe`/`dotnet`/`macho`/`dex`/`magic` stay off until yara-x.
- [ ] **Standalone system-probe needs core-agent artifacts** (`auth_token`, `ipc_cert.pem`) and `remote_agent.configstream.consumer.enabled: false`; not an issue in a normal install, but worth knowing for test environments.

### M1 integration checklist (after wave 1)

Wave 1 branches B, C, D, F are merged and green on `yara/merge-trial`. To do while wiring consumer → `FileReader` → `Pool` → `StructuredReporter` / `Metrics`:

- [x] **Isolate init failures:** `createYaraExecConsumer` currently returns its error from `createEventMonitorModule`, which would take down the whole event monitor (CWS included). Log and continue instead.
- [x] **Check the opened file is the executed one:** on the path fallbacks (`ProcRootFilePath`, other container PIDs), compare the `fstat` inode (and ctime) with the event's `Identity`; on mismatch, don't scan it under the event's identity and don't mark the identity.
- [x] **Root-owned rules only:** the rule loader refuses `rules_dir` and rule files that aren't root-owned or are group/world-writable.
- [x] Wire `FileReaderOpts.ContainerPIDs` to the cgroup resolver.
- [x] Pin down `Stats.Matches` semantics (rule matches vs scans with ≥ 1 match).
- [x] Consider an optional `Close()` on `Scanner`, so the real engine can free compiled rules.
- [x] `Filesystem` is empty for script entries, so the FUSE/NFS bypass doesn't apply to scripts; resolve it or accept it for the PoC.

## 8. Config (proposed)

Keys under `event_monitoring_config.yara`:

| Key | Default | Meaning |
|---|---|---|
| `enabled` | `false` | Turns the consumer on |
| `rules_dir` | unset | Directory of `.yar` files compiled at startup; scanning stays off if unset |
| `chan_size` | `500` | Consumer channel size (exec events) |
| `workers` | `2` | Concurrent YARA scans |
| `queue_size` | `64` | Pending scans before drop |
| `max_file_size` | `64MB` | Larger files are skipped and counted |
| `scan_timeout` | `10s` | Per-file scan limit |
| `identity_cache_size` | `20000` | Identity LRU entries |
| `recheck_ttl` | `24h` | Re-hash an identity after this long, even if it hasn't changed |

These defaults are starting guesses; WS-G tunes them.

## 9. Risks

- **Native dependency in system-probe.** Build, packaging, size and security review may take longer than the code. Mitigation: M1 needs no cgo; start the WS-E conversations during WS-A.
- **C parser on untrusted input, as root (libyara).** A memory-safety bug in a libyara module could be triggered by a crafted binary that any user executes. Mitigation for the PoC: off by default, dev VMs only, minimal module set. Before M4: switch to yara-x.
- **Rules are code.** Whoever can write to `rules_dir` controls what runs against every binary. The loader must refuse rule files (and the directory) that aren't root-owned or are group/world-writable.
- **Exec bursts overflow the channel.** Some execs are missed. The next exec of the same binary retries, so persistent malware is still caught, but one-shot binaries may not be. Watch the drop metric.
- **Short-lived processes.** `/proc/pid/exe` may be gone before the worker runs. The path fallback covers most cases; a deleted binary run by a short-lived process is lost.
- **Cold start cost.** On agent start or a large deploy, every binary is new at once. Queue bounds and drops keep this safe, but the backlog clears slowly.
- **Overlayfs identity.** We haven't verified how mount ID and inode behave across containers that share a layer. If the assumption is wrong, the cost is extra reads only; the sha256 level still dedupes scans.
- **Privileged forging.** Root can forge ctime (clock change, raw disk writes). Out of scope for the PoC; `i_version` is the follow-up.

## 10. Open questions

- Which team owns the consumer long term: CWS or event monitor?
- After the PoC, where do rules come from: bundled, customer-supplied, or Remote Config? (PoC: rules placed by hand in `rules_dir`, e.g. `/etc/datadog-agent/yara.d`, nothing bundled.)
  - *Bundled*: install into `/etc/datadog-agent/<dir>` like CWS's `runtime-security.d`; rule updates need an agent release. Check rule licenses first (signature-base is Detection Rule License 1.1, not BSD).
  - *Remote Config*: like CWS policies; needs hot reload (recompile, swap the scanner), signing, backend work.
  - *Customer-supplied*: files dropped by config management; no work for us.
- What per-host CPU and memory budget is acceptable for the pilot?
- Where should matches go after the PoC (signal, event, or profile annotation)?
