# SUT Discovery: Bug History, Test Strategy, and Product Context
## Focus Areas: 6 (Bug Density), 7 (Test Coverage), 10 (Product Context)
## Commit base: 8ff8f30e10b (main, 2026-05-28)

---

## Areas Examined

- `pkg/logs/` and `comp/logs/` full git history via `git log --all`
- `comp/logs-library/` (sender, client, pipeline, processor)
- `flakes.yaml` for known-flaky signals
- `test/new-e2e/tests/agent-log-pipelines/` for E2E coverage
- 103 unit test files in `pkg/logs/`, 18 in `comp/logs/`, ~30 in `comp/logs-library/`

---

## Focus 6: Bug History and Density

### High-Density Bug Areas (most fix/revert commits)

#### 1. Auditor (offset registry) — correctness under concurrent lifecycle
- `62bf5e55c25` — Fix auditor Flush() race condition during transport restart (AGNTLOG-462). Flush() wrote stale in-memory registry missing payloads already sent to inputChan but not yet consumed. Fix: synchronous Flush() routed through the run() goroutine event loop.
- `a5141ba432c` — Fix flaky TestPartialStop_FlushesRegistryToDisk on ARM64 (#49286). Race between metrics.LogsSent increment and auditor inputChan receipt — test gated on wrong signal.
- `4a842319b51` — (historical) Fixed concurrent access issue in logs auditor
- `1d1b05d054b` — fix(logs): update auditor on disk store and drain queues on shutdown. On shutdown, drain queued payloads to disk to prevent data loss between enqueue and worker processing.
- Pattern: The auditor is a repeated source of race/correctness bugs specifically around the boundary between the sender flushing and the auditor consuming the confirmation channel.

#### 2. File Tailer / File Rotation — data loss and resource leaks
- `86882e6e718` — Fix tailer memory leak and premature halting for Logs fingerprinting (AGNTLOG-327)
- `b95bc0051af` — Fix Nil Pointer Dereference in Tailer DidRotate Functions (AGENT-11735)
- `7964b32e5da` — Prevent panic on stop of rotated tailer during agent shutdown
- `12295d572f4` — Fix Integrations File Rotation on Linux (AGNTLOG-503)
- `eba2ed4cac0` — Read tailer offset before file stat when detecting rotations
- `60c521b9e7d` — Fixed incorrectly truncated logs at log size boundary (AGNTLOG-79)
- `55c63957d9f` — fix(logs/journald): first journal entry dropped when tailing from beginning (#48933)
- `c65053dd108` — (journald) fix seekHead skipping the first journal entry
- Pattern: File rotation is the single most bug-dense area. Issues cluster around: offset read/write ordering, nil pointer on rotation, Windows-specific file handle behavior, fingerprinting/checksum tracking edge cases, and journald seek semantics.

#### 3. Sender / Batch Strategy — shutdown races and memory leaks
- `0d9dfc76f46` — Fix zstd C-memory leak in batch sender error paths (#49626). resetBatch() created new StreamCompressor without closing old one — C-heap leak accumulating to multi-GiB. Present since 7.72.0.
- `f7cf97529ac` — Fix deadlock in destination sender (#11301)
- `90560d965b0` — Fix batch strategy shutdown race (#11205)
- `8f16efe6289` — fix race when stop is called and there are payloads to be flushed (#10533)
- `0bf592ef860` — Fix race condition in logs http destination (#16165)
- `dd88f33e665` — Fix flaky TestBatchStrategySendsPayloadWhenBufferIsOutdated (#22963)
- `891bd2159e3` — fix(logs): prevent multi-reliable duplicate replay and preserve MRF flag in serialization (on feature branch `angel/logs-to-disk`)
- Pattern: The sender is a consistent source of shutdown races and concurrent send correctness bugs. C-memory leaks from zstd compression are invisible to Go GC.

#### 4. Decoder / Multiline / Auto-Multiline — correctness under edge cases
- `046241bfc73` — fix(logs): carry last aggregated line's timestamp through auto-multiline (AGENT-16207) — most recent fix
- `15b1c1c8ae2` — Fix anchored log_processing_rules broken by DetectingAggregator missing TrimSpace (AGNTLOG-617)
- `7687b846b2a` — Fix sampled_count aliasing after pattern table resort (#49226)
- `68a43af22b0` — fix(logs/preprocessor): complete reference tokenizer in FuzzTokenizerCorrectness (#50932)
- `e7149868e46` — Avoid truncating multiline logs aggregated by auto multiline detection (#50095)
- `8163d0e7343` — Auto multiline V2 - reuse cached message. Fixes panic in journald with processing rules.
- `6cd55da9b62` — Multiline Parser Now Marks Truncation and Enforces 900KB Cap
- `e01d3f977ba` — Auto Multiline V2 - Fix double truncation bug (#29882)
- Pattern: Multiline aggregation is continuously buggy. Truncation logic especially has required many revisits (double truncation, wrong boundary, wrong metric count, wrong flag propagation).

#### 5. Launcher Lifecycle — deadlocks and goroutine correctness
- `94d7ccbfc35` — Fix partial logs agent restart (#50828). Unbuffered channels in LogSources left without consumer after partial stop → AddSource() blocks forever.
- `7041f901670` — Fix TCP listener Accept blocking indefinitely on macOS (AGNTLOG-596)
- `3d175eea8f7` — Fix Logs File Launcher Blocking AD (AGNTLOG-74)
- `46251a030f3` — fix tcp launcher/tailer deadlock on agent shutdown (#12316)
- `8d3f55a4eea` — Fix shutdown deadlock in docker socket tailer (#15138)
- `7b02f8fc3b1` — Prevent panic during shutdown of journald and windows event tailers (#33207)
- `dae81c1a82e` — fix(logs): nil-guard stream-logs filter on shutdown (#50952)
- Pattern: Shutdown is the most dangerous lifecycle event: goroutines block on closed or unconsumed channels, nil dereferences appear on partially-stopped components.

#### 6. Container / K8s Logs — duplication and tags
- `c88981fd067` — EKS duplicate log fix (#40129). Kubelet API bug — agent added check to skip logs before lastSince timestamp.
- `7991059a638` — Fix duplicate tags in TCP/UDP logs (#29780)
- `9040528765e` — fix: rearranged logic so service and source properly populated (#40021)

#### 7. MRF (Multi-Region Failover) / Additional Endpoints
- `f6edf45a2fb` — Fix connection_reset_interval not applied to additional/MRF endpoints (AGNTLOG-461)
- `891bd2159e3` — fix(logs): prevent multi-reliable duplicate replay (on feature branch)
- `93ec749aafc` — Fix dual shipping recovery in logs client (CONTINT-3776)

### Suspiciously Quiet Areas (low churn — possibly undertested)
- `pkg/logs/service/` — 3 commits since 2025-01-01, all non-logic (Bazel, linter). The Services struct (container service registry) has no fix commits in recent history.
- `pkg/logs/schedulers/channel/` — minimal recent changes.
- `comp/logs/adscheduler/` — new component with zero test files in def/fx/impl.
- `comp/logs/streamlogs/` — zero test files across all sub-packages.
- `pkg/logs/launchers/channel/launcher.go` — no tests, no recent fixes; the `Stop()` sends on `l.stop` (unbuffered) after closing `sourcesDone`, which is the same pattern that triggered the partial-restart deadlock fixed in `94d7ccbfc35`.

### Recent High-Activity Feature (new risk area)
- Disk retry (`angel/logs-to-disk` branch): Adds payload serialization to disk during network outages. Multiple fixes already (duplicate replay, auditor offset coordination, drain-on-shutdown). Not yet merged to main, but the supporting auditor/sender changes already are.

---

## Focus 7: Existing Test Strategy

### Test File Inventory

#### Unit Tests — pkg/logs/ (103 files)
| Area | Test Files | Key Coverage | Mocking Level |
|------|-----------|-------------|---------------|
| `tailers/file/` | tailer_test.go, tailer_integration_test.go, tailer_windows_test.go, fingerprint_test.go | StopAfterFileRotationWhenStuck (backpressure + rotation), goroutine leak (goleak), encoding variants, rotation detection | Real disk, real channels. No network. |
| `tailers/journald/` | tailer_test.go, docker_test.go | MockJournal struct mimics sdjournal; seekHead, filter, structured msgs | Fully mocked journald (systemd build tag required) |
| `tailers/socket/` | stream_tailer_test.go, datagram_tailer_test.go, syslog_*.go | TCP/UDP parsing, syslog framing | Uses real local TCP/UDP sockets |
| `launchers/file/` | launcher_test.go, position_test.go, provider/file_provider_test.go | File scanning, position tracking, ordering | Real FS, no send-side integration |
| `launchers/integration/` | launcher_test.go | Flaky historically; basic lifecycle | Partially mocked |
| `launchers/container/tailerfactory/` | api_test.go, factory_test.go, file_test.go, socket_test.go, whichtailer_test.go | Container log source routing decisions | Mocked container runtime |
| `internal/decoder/` | decoder_test.go, line_handler_test.go, auto_multiline_handler_test.go, single_line_handler_test.go | Multiline aggregation, line parsing, handler transitions | No real I/O |
| `internal/decoder/preprocessor/` | 12 test files + benchmarks | Adaptive sampler, JSON aggregator, tokenizer, labeler, pattern table, timestamp detection | Pure unit tests |
| `internal/parsers/{docker,k8s,syslog,integrations}/` | 6 test files + 6 fuzz test files | Parser correctness, edge cases | Fuzz-tested with real Go fuzzer |
| `schedulers/ad/` | scheduler_test.go | AD scheduler integration | Mocked AD |
| `sources/` | sources_test.go, source_test.go, config_source_test.go | Source lifecycle | Pure unit |
| `diagnostic/` | message_receiver_test.go | Message diagnostics | Pure unit |

#### Unit Tests — comp/logs/ + comp/logs-library/ (48 files)
| Area | Test Files | Key Coverage |
|------|-----------|-------------|
| `comp/logs/auditor/impl/` | auditor_test.go, api_v0/v1/v2_test.go, registry_writer_test.go | Offset persistence, registry recovery, liveness, Flush() correctness |
| `comp/logs/agent/agentimpl/` | agent_test.go, agent_restart_test.go, agent_core_init_test.go | Full restart cycle, concurrent restart serialization, HTTP upgrade, TCP→HTTP failover, flush-on-restart |
| `comp/logs-library/sender/` | worker_test.go, batch_strategy_test.go, batch_test.go, destination_sender_test.go, stream_strategy_test.go | Dual/single destination, shutdown drain, deadlock, MRF payloads |
| `comp/logs-library/client/http/` | destination_test.go, worker_pool_test.go | HTTP retries (500, 429, 404), secrets refresh on 403, dropped metric, timeout |
| `comp/logs-library/client/tcp/` | destination_test.go, connection_manager_test.go | HA failover, drop metric |
| `comp/logs-library/pipeline/` | provider_test.go, provider_failover_test.go, provider_failover_integration_test.go | Channel distribution, failover routing, concurrent throughput, shutdown-under-load |
| `comp/logs-library/processor/` | processor_test.go, encoder_test.go | Exclusion/inclusion rules, masking, truncation, encoder correctness |
| `comp/logs-library/metrics/` | 4 test files | Pipeline/capacity/utilization monitors |

#### E2E Tests — test/new-e2e/tests/agent-log-pipelines/
| Suite | What it tests | What's missing |
|-------|--------------|----------------|
| `linux-log/file-tailing/` | Log collection, permission denied → recovery, file rotation (delete+recreate only) | Copy rotation (cp+truncate), inode reuse, high-throughput rotation under backpressure |
| `linux-log/journald/` | Basic journald collection, agent restart picks up service | Volume, cursor recovery after agent kill, first-entry race |
| `linux-log/integrations/` | Integration check log files | Rotation, encoding edge cases |
| `listener/` | TCP and UDP listener log collection in Docker | TLS failures, reconnect after server restart |
| `k8s-logs/` | Single log + metadata in Kind, long line handling | CCA (container_collect_all), pod restart mid-stream, namespace scaling |
| `k8s-logs/file_tailing_cca_off_test.go` | CCA-off behavior | — |
| `windows-log/file-tailing/` | Windows file tailing basics | Log file handles during rotation, event log |

### What Tests Cover Well
1. **Decoder/parser correctness** — well-covered with unit + fuzz tests across all parser types.
2. **Adaptive sampler** — comprehensive unit test suite (22+ test functions), recently added benchmarks.
3. **Auditor persistence** — recovery, registry cleanup, v0/v1/v2 format migration, Flush() correctness all tested.
4. **Agent restart lifecycle** — the restart test suite covers TCP→HTTP upgrade, concurrent restarts, rollback, flush-on-stop.
5. **HTTP/TCP destination send + retry** — unit tests cover 429/500/404 retries, timeouts, dropped metric, deadlock guard.
6. **Batch strategy** — buffer-full flush, outdated flush, graceful shutdown (non-blocking).

### What Tests Do NOT Cover (Antithesis Value)

1. **Backpressure → rotation loss**: The existing `TestStopAfterFileRotationWhenStuck` tests that a blocked tailer stops correctly, but does NOT test what happens to log lines written to the file while the tailer was blocked (lines written after rotation point). E2E file rotation tests use only delete+recreate, not copy+truncate (logrotate default), so the fingerprint/offset coordination for that case is untested end-to-end.

2. **Network partition during send**: HTTP destination tests use httptest.Server — not real network chaos. There are no tests for: packet-loss mid-payload, TLS handshake interruption, partial write on TCP connection, or connection reset during retry backoff.

3. **Kill -9 / OOM recovery**: The auditor registry test (`TestAuditorFlushesAndRecoversRegistry`) tests the happy path: write → stop → restart reads offset. It does NOT test: ungraceful kill (offset not flushed), concurrent writes during shutdown, corrupt registry file, or the race between Flush() and the next Start().

4. **Multi-pipeline channel saturation + failover under load**: `TestConcurrentHighThroughput` uses only 2 pipelines and a mock sender (no real network). No test verifies that when one pipeline's destination is slow (real network stall), the failover router correctly distributes messages and the lagging pipeline does not cause global backpressure.

5. **Journald cursor recovery on restart**: The journald unit tests use a MockJournal. There is no E2E test verifying that after an agent crash (not graceful stop), the journald tailer resumes from the correct cursor without duplicates or gaps.

6. **Container log deduplication under kubelet API delay**: The EKS duplicate fix (`c88981fd067`) has no E2E regression test in the logs pipeline E2E suite.

7. **Windows file rotation under file locks**: The integration test `tailer_windows_test.go` is limited and does not simulate concurrent write+rotation.

8. **Adaptive sampler under concurrent pattern table resort**: Fix `7687b846b2a` was for sampled_count aliasing during sort — no concurrent stress test for the pattern table.

9. **Disk retry serialization correctness**: The `diskretry` subsystem (on feature branch) has newly introduced serialization format. The MRF duplicate replay fix (`891bd2159e3`) added unit tests but there are no integration tests covering: serialize → crash → deserialize → replay without duplicates.

10. **Full pipeline latency under sustained load**: No test measures end-to-end latency from file write to fakeintake arrival at sustained throughput, so degradation under load is invisible.

### Known Flaky Signals
- `flakes.yaml` at repo root has zero log-pipeline entries. This indicates either: (a) logs tests are currently clean, or (b) flaky logs tests are fixed (confirmed: several explicitly fixed in commits above) or disabled.
- Historical flaky tests (now fixed): `TestPartialStop_FlushesRegistryToDisk` (ARM64 race), `TestTCPMTLSOptionalAcceptsClientCert` (macOS arm64), integration launcher test, batch strategy timer test, file tailer encoding test, eventlog integration tests.
- The file provider ordering test `Multiple Directories - Out of order input` is explicitly skipped with a FIXME comment (line 645, `pkg/logs/launchers/file/provider/file_provider_test.go`). This represents a known correctness gap in `applyReverseLexicographicalOrdering` when globs return paths out of lexicographical order.

---

## Focus 10: Product Context

### Production-Critical Log Sources (by usage volume)

1. **File tailing** — dominant production case. Every host with a traditional daemon, application server, or systemd service uses this. The file launcher scans for matching globs, creates one tailer per file, and the tailer blocks on the pipeline channel. A blocked pipeline (network outage) causes the tailer to stall and risks rotation loss when logrotate moves the file while the tailer is blocked with the old fd.

2. **Container / Kubernetes file tailing** — second most common production use case. Agent runs as DaemonSet and tails `/var/log/pods/...`. The container launcher uses tailerfactory to pick between Kubernetes file API and direct file. High churn: pods start/stop continuously, requiring the auditor to maintain per-container offsets. The CCA (container_collect_all) path adds all running containers without explicit config.

3. **Journald** — common on systemd hosts (most modern Linux distributions). Used for system logs, SSH, kernel messages. Journald has its own cursor-based seek; the first-entry-dropped bug (`55c63957d9f`, `c65053dd108`) was a real production gap.

4. **TCP/UDP listener** — used for syslog forwarding from network devices, legacy applications, and log aggregators. The TCP listener now supports TLS with mTLS and automatic certificate rotation (`09a2c69a534`). The UDP listener is connectionless and drops on overload — no backpressure possible.

5. **Windows Event Log** — important for Windows-hosted workloads. Separate tailer subsystem. The event log test has historically been disabled (`e6cc2fbc9dc`) and re-enabled.

6. **Integration check logs** — Python checks can emit logs; the integration launcher tails from a shared log directory. This path has its own rotation bug history (`12295d572f4`).

### User-Visible Failure Modes

| Failure Mode | What User Sees | Internal Mechanism | Detected By |
|-------------|---------------|-------------------|-------------|
| **Log loss during file rotation** | Gaps in log timeline in Datadog UI | Backpressure blocks tailer → file rotated → old fd abandoned | `logs.dropped` metric; gaps visible in explorer |
| **Duplicate logs after agent restart** | Same log lines appear 2×+ in Datadog | Auditor flush incomplete before kill → offset rewound | User report; no automatic dedup at intake |
| **Logs stop flowing (silent)** | No logs in Datadog, no alert | Sender stuck in retry loop (429/500), channel full, tailer blocked | Agent health status; `logs.dropped` metric |
| **Truncated multiline log** | Multi-line stacktrace split or truncated | Buffer exceeded 900KB cap or timer expired mid-aggregate | Truncation tag in log metadata |
| **Tag missing from logs** | Logs arrive without container/k8s tags | Tagger not warmed up at collection start, or AD tag completeness not met | No metadata on log event |
| **Agent OOM/crash** | Agent process killed; monitoring gap | Pipeline channel full, C-heap leak (zstd), or memory pressure from pattern table | OS process death; restart detected by supervisor |
| **Adaptive sampling drops important logs** | Security/error logs missing | Pattern matching incorrectly classifies log as low-priority | `bytes_dropped` telemetry; user complaint |
| **Duplicate logs during EKS pod restart** | Same container boot logs appear 2× | Kubelet API timestamp sub-second precision bug | User visible in log explorer |

### Which Workflows Matter Most
- **File tailing + logrotate under backpressure** is the highest-risk production scenario and the one most vulnerable to data loss without fault injection.
- **Agent restart / transport upgrade (TCP→HTTP)** is a common operational event during upgrades. The restart pathway has multiple recent fixes and is an ongoing correctness concern.
- **Journald cursor recovery after ungraceful shutdown** matters for any host using systemd.
- **Container log collection at pod churn rate** (Kubernetes deployments doing rolling updates) exercises the per-container tailer lifecycle at high frequency.

---

## Assumptions and Open Questions

1. The disk-retry feature (`angel/logs-to-disk`) is not yet on main. Serialization correctness bugs would be in a new code path with minimal test coverage for adversarial scenarios.

2. The adaptive sampler is not enabled by default in all configurations — need to confirm what fraction of production agents use it before treating sampler bugs as P0.

3. The `flakes.yaml` absence of log entries may be misleading: some known-flaky tests were fixed and removed from the file rather than remaining listed. Historical flakiness of `TestPartialStop_FlushesRegistryToDisk` is a strong signal of a real race that existed for a significant time.

4. The channel-launcher (`pkg/logs/launchers/channel/launcher.go`) has no tests and uses the same unbuffered channel pattern (`l.stop <- struct{}{}`) that caused the partial-restart deadlock in LogSources. This is a candidate for the same class of bug.

5. The file provider ordering FIXME (`applyReverseLexicographicalOrdering`) affects which files are tailed first when the glob returns many files. In production, if this ordering is wrong, the agent might tail older files first, causing temporal reordering of events.
