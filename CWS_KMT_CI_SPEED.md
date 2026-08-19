# Making `kmt_run_secagent_tests_*` (`cws_host`) faster

Investigation notes, 2026-08-04. Branch `daniel.mercier/faster-ci`.

The follow-up implementation plan for test grouping is in `CWS_TEST_GROUPING_PLAN.md`.

Reference job: <https://gitlab.ddbuild.io/datadog/datadog-agent/-/jobs/1916277389>
(`kmt_run_secagent_tests_x64: [amazon_6.12, cws_host]`, pipeline `128540308`, commit
`809fc228c854`). Job wall clock **57.3 min**, ended in a `-test.timeout` panic.

## 1. Where the time actually goes

Job breakdown from the trace sections:

| phase | time |
|---|---|
| pod prep + git clone + artifact download | ~2 min |
| **the test binary** | **55.2 min** |
| after_script (junit upload, tagging) | 13 s |

Nothing to win in the wrapper — it is all the suite.

The suite runs with `-test.timeout=55m0s` (3300 s) and reported:

```
DONE 651 tests, 37 skipped, 2 failures in 3310.661s
ERROR rerun aborted because previous run had a suspected panic and some test may not have run
```

**It lost by 10 seconds.** `TestTracerMemfd` is the victim, not the cause: it was the test
in flight when the global timeout fired, so it absorbed the whole elapsed time
(`FAIL: pkg/security TestTracerMemfd (3310.66s)`) and dumped every goroutine. The panic
also made gotestsum abort the reruns, so the `--rerun-fails=2` budget was thrown away.

196 top-level tests actually execute (651 counting subtests, 37 skipped), strictly serial
in one process. Splitting the 3264 s of accounted test time:

| bucket | time | share |
|---|---|---|
| **tearing down the previous eBPF probe** | **1511 s** | **46 %** |
| building/starting the new probe + those tests' bodies | 439 s | 13 % |
| bodies of tests that reused the module | 1274 s | 39 % |

### How the first row was measured

43 tests emit `module_tester_linux.go:751: Instantiating a new security module`. For each,
the gap between the test's test2json `run` event and that log line is:

```
median 34.3 s   mean 35.1 s   min 32.7 s   max 54.8 s   sum 1511 s
```

Everything *after* that line — eBPF load, attaching ~76 probes, procfs snapshot, **and the
test body** — is only ~5–6 s (median 6.2 s). The one cheap case is `TestAcceptEvent` at
2.0 s: it is the first test in the run, so there is no previous module to destroy.

That pre-line window is dominated by `testMod.cleanup()`:

- `pkg/security/tests/module_tester_linux.go:743` — `testMod.cleanup()` when static opts differ
- `pkg/security/tests/module_tester_linux.go:1012` — `cleanup()` = `eventMonitor.Close()`
- `pkg/eventmonitor/eventmonitor.go:141` — `Probe.Stop()`, consumer `Stop()`, `Probe.Close()`
- `pkg/security/probe/probe_ebpf.go:2529` — `Stop()`: `Manager.StopReaders(CleanAll)` + `wg.Wait()`
- `pkg/security/probe/probe_ebpf.go:2546` — `Close()`: `Manager.Stop(manager.CleanAll)`, `Erpc.Close()`, `Resolvers.Close()`

**~25 of the 55 minutes are spent destroying eBPF probes.**

Reload cost per test (`total / pre-instantiate / post-instantiate`), worst first:

```
TestActionKillDisarm                    98.1   54.8   43.3
TestRemediationCustomEvents             87.2   53.1   34.1
TestActionHash                          75.1   53.7   21.4
TestCaptureAllSyscallErrorsDisabled...  61.7   35.8   25.9
TestActionKillExcludeBinary             60.8   54.4    6.4
TestActionKillRuleSpecific              59.4   52.8    6.6
TestFilterRuntimeDiscarded              58.9   33.7   25.2
TestSELinux                             55.9   40.0   15.9
TestSelfTests                           55.6   34.1   21.4
...                                     ~38-45 ~33-35  ~5-8   (33 more tests)
TestAcceptEvent                         11.1    2.0    9.1   (first test, no teardown)
```

## 2. Angle 1 — the metal instance is 3x oversubscribed (cheapest, biggest, no code)

From `stack.output` of `kmt_setup_env_secagent_x64` (job 1916277264):

- **76 microVMs, each `ddvm-4-12288` (4 vCPU / 12 GiB), all on ONE m5d.metal**
  (96 vCPU / 384 GiB) at `10.255.104.148`. `x86_64` has a single `ip` key.
- 304 vCPU requested on 96 → **3.2x CPU**; 912 GiB on 384 → **2.4x memory**.
- vmset tags on that one box: `cws_host` 25, `cws_peds` 25, `cws_docker` 22, plus
  `cws_req`/`cws_ad`/`cws_el`/`cws_el_ns` 1 each.
- arm64 is worse: ~56 VMs x 4 vCPU on m6gd.metal (64 vCPU) → **~3.5x**.
- Sizing comes from `.gitlab/test/kernel_matrix_testing/common.yml:187`
  (`kmt.gen-config --memory=12288`) and `DEFAULT_VCPU = "4"` in `tasks/kmt.py:109`.

Corroboration: the *identical* `cws_host` suite ranges **29.7 → 61.1 min** across
platforms in this one pipeline. That spread is contention, not kernel differences.

Longest secagent KMT jobs in pipeline 128540308 (48 job-hours total for the stage):

```
61.1 failed   kmt_run_secagent_tests_x64:   [ubuntu_26.04, cws_host]
57.3 failed   kmt_run_secagent_tests_x64:   [amazon_6.12,  cws_host]   <- reference job
48.1 failed   kmt_run_secagent_tests_arm64: [ubuntu_26.04, cws_host]
48.0 success  kmt_run_secagent_tests_arm64: [amazon_6.12,  cws_host]
45.4 success  kmt_run_secagent_tests_arm64: [fedora_37,    cws_host]
...
29.7 success  kmt_run_secagent_tests_x64:   [oracle_9.3,   cws_host]
 3.1 success  kmt_run_secagent_tests_x64_required: [ubuntu_25.10, cws_req]
```

Options:

- second metal instance per arch (needs a real change: VM→instance assignment lives in
  the pulumi scenario, and jobs find their box via `.get_instance_ip_by_type`, which
  filters by pipeline + component + instance type and expects one IP);
- larger instance type;
- stagger `cws_docker` (17–40 min, 22 VMs) so it does not overlap `cws_host` end to end.
  `cws_peds` finishes in 3 min, so it only contends at the start.

**Cheap experiment that must come first:** run one `cws_host` VM alone on a metal box and
compare against 55 min. That single measurement tells us how much of the 34 s teardown is
CPU starvation vs. genuine kernel work — i.e. whether this is Angle 1 or Angle 2.

## 3. Angle 2 — kill the 25 min of teardown

### 2a. Measure it first

Instrument the three phases of `EventMonitor.Close()`: `Probe.Stop()`
(`StopReaders` + `wg.Wait`), `Manager.Stop(manager.CleanAll)`, `Resolvers.Close()`.

Hypothesis (**unverified**): it is detaching the ~76 kprobes. Each `unregister_kprobe`
does a `synchronize_rcu_tasks()`, ~0.4 s on a contended 4-vCPU VM, x76 ≈ 30 s. Fits:

- the suspiciously flat 34 s regardless of what the test did;
- attach being nearly free while detach is not;
- newest kernels being slowest — `selectFentryMode` logs
  `disabling fentry and falling back to kprobe mode: fentry disabled on kernels >= 6.11`,
  and `ubuntu_26.04` (61 min) + `amazon_6.12` (57 min) are the two worst jobs.

The goroutine dump in the reference job shows `UpdateActivatedProbes` with `0x4c` = 76
probes.

### 2b. If it is RCU grace periods

Add `rcupdate.rcu_expedited=1` to the KMT microVM kernel cmdline. One line of infra, no
test-code risk, potentially ~20 min off this job — and it would help the system-probe KMT
jobs too.

### 2c. Don't tear down synchronously

Detach the old manager in the background while the next one loads, or skip `CleanAll` in
tests and let process exit reclaim. Riskier: two attached probe sets briefly coexist, so
the old one keeps feeding perf buffers nobody reads.

### 2d. Free win — `testOpts.Equal` is `reflect.DeepEqual`

`pkg/security/tests/testopts.go:112` compares the whole struct, which contains func fields
(`preStartCallback`, `ruleMatchHandler`, `tagger`). **Non-nil func values are never
`DeepEqual`**, so every test setting one forces a reload *and* forces the next test to
reload as well (stored non-nil vs. new nil).

Affected: `TestReplay` (`replay_test.go`, 4 uses), `TestEventMonitor` +
`TestEventMonitorNoEnvs` (`eventmonitor_test.go`), `TestTracerMemfd`
(`tracer_memfd_test.go`), plus `security_profile_test.go` (11 `tagger:` uses, skipped in
`cws_host`) — and each one's successor. ~4–8 reloads ≈ 3–5 min. Fix: compare only the
value fields.

## 4. Angle 3 — order tests by config: 41 switches → 20

Static analysis of `withStaticOpts(testOpts{...})` literals across the 196 executed
top-level tests: **only 20 distinct configurations, and 156 tests use the default.** Yet
the run performs 41 config switches, because non-default tests are scattered through
source order (Go runs tests in file order, then declaration order) — an isolated
non-default test costs two reloads: in, then back to default.

```
156 <default>
 14 {networkIngressEnabled:true}                 TestDNSResolver, TestFailedDNS*, TestAWSIMDS*...
  7 {networkRawPacketEnabled:true}               TestRawPacket*, TestRemediationCustomEvents...
  2 {disableMapDentryResolution:true}            TestDentryPathERPC, TestDentryInvalidation
  2 {dnsPort:DNSPort}                            TestDNSResponse, TestDNSResponseDiscarder
  1 x16 singletons                               TestSelfTests, TestCapabilitiesEvent,
                                                 TestCaptureAllSyscallErrors, TestDentryPathMap,
                                                 TestNetworkFlowSendUDP4, TestOnDemandOpen,
                                                 TestOpenApproverZero, TestFilterRuntimeDiscarded,
                                                 TestActionKillDisarm, TestActionKillExcludeBinary,
                                                 TestEventMonitor(+NoEnvs), TestReplay,
                                                 TestTracerMemfd, TestFilterOpenLeafDiscarderActivityDump
```

The two big blocks are nearly contiguous but get split: the 14 `networkIngressEnabled`
DNS/IMDS tests are broken by `TestDNSResponse` (`dnsPort`), and the 7
`networkRawPacketEnabled` tests by `TestRemediationCustomEvents`.

Perfect grouping = 20 reloads → saves ~21 x 35 s ≈ **12 min today** (only ~2 min once
Angle 2 lands; the two compound rather than overlap). Options:

1. relocate/rename test funcs so file + declaration order groups them — fragile;
2. run the suite as several `-test.run` passes grouped by config — one module build per
   pass, and doubles as Angle 4;
3. restructure non-default tests as subtests under one parent per config.

Related: many of the 20 configs are plain config *values* (rate limiters, `dnsPort`,
disarmer thresholds, `capabilitiesMonitoringEnabled`, `captureAllSyscallErrorsEnabled`),
not different program sets. Making those runtime-reconfigurable would let those tests
reuse the module entirely instead of reloading.

## 5. Angle 4 — shard `cws_host` (needs Angle 1 first)

The mechanism already exists. `test/new-e2e/system-probe/test-runner/files/cws_host.json`
is an empty filter:

```json
{ "filters": { "*": { "exclude": false } } }
```

while `cws_peds.json` / `cws_ad.json` / `cws_req.json` use `run-only` lists, and
`test/new-e2e/system-probe/test-runner/main.go:158` turns those into `-test.run`.

Splitting into 3 config-grouped shards gives ~15–20 min each — but it adds ~50 microVMs
to a box already at 3.2x, so it only works paired with more hardware.

Larger total-cost lever: `cws_host` runs the full suite on **25 x86 + 18 arm64 kernels**
(`.gitlab/test/kernel_matrix_testing/security_agent.yml:140` and `:332`), and this
pipeline burned **48 job-hours** on the secagent KMT stage. Full suite on ~6
representative kernels per PR + full matrix nightly would cut that ~4x.

## 6. Angle 5 — the timeout cliff (safety net, do now)

`pkg/security.*` → `55 * time.Minute` at
`test/new-e2e/system-probe/test-runner/main.go:75`. The GitLab job timeout is already
1 h 30 m (`.gitlab/test/kernel_matrix_testing/security_agent.yml:117`), so bumping the
test timeout to ~75 m costs nothing and converts "panic → reruns aborted → whole job
lost" into real results while the actual fixes land.

Note the comment at `main.go:68` — `TEST_TIMEOUTS` in `tasks/system_probe.py:67` is meant
to mirror this map, but currently has no `pkg/security` entry.

Separately: a global `-test.timeout` means one hung test burns the entire budget *and*
kills the reruns. A per-test watchdog would contain that.

## 7. Angle 6 — individual long poles

Of the 1274 s of genuine test-body time (tests that reused the module):

```
160.5 TestReplay
 83.1 TestEventTruncatedParents
 55.4 TestEventHeartbeatSent
 39.9 TestPTraceEvent
 39.0 TestFilterOpenAUIDEqualApprover
 36.4 TestRawPacketActionProcessScopeWithSignature
 35.2 TestCustomEventContainer
 34.9 TestTCFilters
 31.8 TestActionKillContainerWithSignature
 31.4 TestFilterDiscarderRetention
```

Several look like fixed interval waits (heartbeat period, discarder retention) that tests
could shorten via config. ~5 min available across a long tail.

## 8. Suggested order

1. **Angle 1 measurement** (one `cws_host` VM alone on a metal box) + **Angle 5** timeout
   bump — hours, no risk, and the measurement decides everything below.
2. **Angle 2d** (`testOpts.Equal`) + **Angle 3** (grouping) — ~15 min saved, contained diffs.
3. **Angle 2a → 2b** — the real prize (~20–25 min). Do not design before measuring.
4. **Angle 4** (sharding / matrix trimming) and **Angle 6** (long tail).

## 9. The Angle 2 experiment (implemented, commit `56598e7c137`)

Instrumentation is in place to attribute the 34 s and to A/B the RCU hypothesis in a single
pipeline. Everything is gated by env vars, so it is inert unless a run config asks for it.

### What was added

| file | change |
|---|---|
| `pkg/security/seclog/logger.go` | `StartPhase(name) func()` — gated phase timer, reports at **warn** level (the suite's default level, so it lands in the log). `PhaseProfilingEnabled()`. Driven by `DD_CWS_PHASE_PROFILING`. |
| `pkg/eventmonitor/eventmonitor.go` | `Close()` split into `Probe.Stop` / `consumers.Stop` (per consumer ID) / `cancel+wait` / `Probe.Close`. |
| `pkg/security/probe/probe_ebpf.go` | `Stop()` split into `Manager.StopReaders` / `cancel+wait`. `Close()` split into `rawPacketCollections` / `nameMappings+telemetry` / `Manager.Stop` / `Erpc.Close` / `Resolvers.Close`, plus a line reporting how many probes are running vs. total right before `Manager.Stop`. |
| `pkg/security/tests/module_tester_linux.go` | `testPhase()` times the `newTestModule` steps that make up the pre-rebuild window (`newSimpleTest`, `setTestPolicy`, `cmdWrapper`, `testMod.cleanup`, `genTestConfigs`) plus `eventMonitor.Init` / `Start`, via `t.Logf`. `logCPUPressure()` samples `/proc/loadavg` + `/proc/pressure/cpu` + `NumCPU` immediately before and after teardown. |
| `pkg/security/tests/main_linux.go` | `applyRCUExpedited()` in `preTestsHook`: when `DD_CWS_RCU_EXPEDITED` is set, writes `/sys/kernel/rcu_expedited=1` and `/sys/kernel/rcu_normal=0`. Both knobs are root-writable sysfs files, so no kernel cmdline change is needed. |
| `test/new-e2e/.../files/cws_teardown.json` | 10 reload-forcing tests, `DD_CWS_PHASE_PROFILING=1`. |
| `test/new-e2e/.../files/cws_teardown_rcu.json` | same 10 tests, `+ DD_CWS_RCU_EXPEDITED=1`. |
| `.gitlab/test/kernel_matrix_testing/security_agent.yml` | `kmt_run_secagent_tests_x64_teardown`, matrix `{amazon_6.12, ubuntu_22.04} x {cws_teardown, cws_teardown_rcu}`. Wired into `kmt_secagent_tests_join2_x64` so cleanup cannot destroy the metal instance mid-run. |

### Why those 10 tests

`^TestDentryPathERPC$`, `^TestDentryPathMap$`, `^TestOnDemandOpen$`,
`^TestNetworkFlowSendUDP4$`, `^TestNetDevice$`, `^TestCapabilitiesEvent$`,
`^TestCaptureAllSyscallErrors$`, `^TestOpenApproverZero$`, `^TestFilterRuntimeDiscarded$`,
`^TestSelfTests$`.

Their static configs are **pairwise distinct**, so every one of them forces a reload
regardless of what `-test.run` filtering does to adjacency — 10 reloads, ~9 measured
teardowns, in a job of roughly 8–10 min instead of 55. Anchors matter: `-test.run` matches
each element as an unanchored regexp, so `TestHardLink` would also pull in
`TestHardLinkExecsWithMaps`.

### Deliberate choice: keep the full matrices

The new jobs add only 4 microVMs (76 → 80 on the x86 box) and run **alongside** the
existing `cws_host`/`cws_peds`/`cws_docker` matrices. Contention is therefore identical to
production, which is what isolates Angle 2 from Angle 1. Trimming the matrices to get a
quiet box is the *separate* Angle 1 experiment — do it as a second pipeline, don't conflate
the two.

### Reading the results

Pull `testjson-x86_64-<tag>-cws_teardown{,_rcu}.tar.gz` from each job (see §10) and grep
`out.json` for `[phase]`. Expected decision table:

| observation | conclusion |
|---|---|
| `EBPFProbe.Close/Manager.Stop` ≈ 30 s of the 34 s | teardown is probe detach; go to the RCU arm |
| `_rcu` variant drops `Manager.Stop` to a few seconds | RCU grace periods confirmed → ship `rcu_expedited` (Angle 2b) as the fix |
| `_rcu` changes nothing | not RCU; instrument inside ebpf-manager's `Manager.Stop` next |
| time is in `Resolvers.Close` or `consumers.Stop/<id>` | product-code teardown bug, not a kernel cost |
| time is in `newTestModule/cmdWrapper` | it is `newDockerCmdWrapper`, not the probe at all |
| `loadavg` >> 4 with high PSI `some` | large contention component → Angle 1 first |

### Pre-flight already done locally

- `dda inv -e security-agent.build-functional-tests` — compiles; golangci-lint 0 issues
- `dda inv -e linter.go --targets=./pkg/eventmonitor,./pkg/security/probe,./pkg/security/seclog` — passed
- `dda inv -e linter.gitlab-ci` — passed. Note `kmt_secagent_tests_join1_x64` was already at
  the GitLab 50-`needs` ceiling (25 + 1 + 2 + 22), which is why the new job hangs off join2.
- `dda inv -e linter.gitlab-ci-jobs-codeowners`, `linter.gitlab-change-paths` — passed
- `kmt.gen-config` derives vmsets from this yml, so the new test sets provision VMs
  automatically: `cws_teardown -> 2`, `cws_teardown_rcu -> 2`
- `testsuite -test.list` resolves all 10 anchored names to exactly 10 tests

### To trigger

```bash
git push -u origin daniel.mercier/faster-ci
gh pr create --draft --title "[DO NOT MERGE] Measure CWS eBPF probe teardown cost in KMT" --body "..."
```

Then read the four `kmt_run_secagent_tests_x64_teardown` jobs. KMT jobs run on
`.on_security_agent_changes_or_manual`, and this branch touches `pkg/security/**`, so they
fire automatically.

## 10. RESULTS — Angle 2 measured (pipeline 129070096, 2026-08-05)

Four arms, all green. 80 microVMs on one m5d.metal (3.33x CPU / 2.50x memory oversubscribed),
~270 running probes out of ~532 in every arm, **kprobe mode on both platforms** (so no fentry
confound: 5.15 logs `fentry enabled but not fully supported`, 6.12 logs `fentry disabled on
kernels >= 6.11`).

Median `EBPFProbe.Close/Manager.Stop` per module teardown:

| platform | kernel | baseline | `rcu_expedited=1` | speedup |
|---|---|---|---|---|
| ubuntu_22.04 | 5.15 | 7.11s | **0.69s** | **10.3x** |
| amazon_6.12 | 6.12 | **34.28s** | 20.02s | 1.71x |

Job wall clock: 316s / 273s (ubuntu), 567s / 420s (amazon).

### Finding 1 — the 34s is confirmed, and it is `Manager.Stop`

The production reference job measured a 34.3s median pre-instantiate window on amazon_6.12,
inferred from test2json timestamps (§1). Direct instrumentation on the same platform gives
`testMod.cleanup` = **34.38s** and `Manager.Stop` = **34.28s**. The inference was correct and
the cost is specifically detaching the eBPF probes — not config generation, not the Docker
cmd wrapper, not resolvers. Everything else in the teardown path is ~0.00s:
`Probe.Stop`, `StopReaders`, `cancel+wait`, `consumers.Stop/CWS`, `Resolvers.Close`,
`Erpc.Close`, `rawPacketCollections`, `newSimpleTest`, `setTestPolicy`.

### Finding 2 — it is NOT contention. Angle 1 is exonerated for this cost

The CPU-pressure samples settle Angle 1 vs Angle 2:

```
before teardown  load= 0.16   PSI some avg10=1.00%
after  teardown  load= 5.59   PSI some avg10=0.03%
before teardown  load=14.43   PSI some avg10=0.65%
```

Load average climbs into double digits **while CPU pressure falls to near zero**. That is
tasks accumulating in uninterruptible sleep — blocked in the kernel — not competing for CPU.
The 34s is genuine kernel blocking in probe detach and would be 34s on an idle box.

Oversubscription is still real (§2) and still worth fixing for its own sake, but it does not
explain the dominant cost of the cws_host suite. Angle 2 is the prize; Angle 1 drops down the
list.

### Finding 3 — `rcu_expedited` is a 10x fix on <= 6.10 and only 1.7x on 6.12

The A/B on 5.15 is textbook: `rcu_expedited` moves **only** the detach phase, and every other
phase matches to two decimal places (`eventMonitor.Init` 6.99 vs 7.16, `Start` 2.06 vs 2.08,
`cmdWrapper` 0.24 vs 0.29, `genTestConfigs` 0.08 vs 0.08). Probe detach is RCU-grace-period
bound, as hypothesized in §3.

On 6.12 it only gets 34.28s -> 20.02s. `/sys/kernel/rcu_expedited` expedites
`synchronize_rcu()`, but kprobe removal also waits on **RCU-Tasks**, a separate flavor with its
own grace-period machinery. The candidate knobs exist on 6.x but are **read-only**, i.e.
boot-param only — so chasing the 6.12 residual needs a microVM kernel cmdline change and loses
the "no infra change" advantage:

```
/sys/module/rcupdate/parameters/rcu_task_lazy_lim        32   r--r--r--
/sys/module/rcupdate/parameters/rcu_task_enqueue_lim      1
/sys/module/rcupdate/parameters/rcu_tasks_rude_lazy_ms   -1   r--r--r--
/sys/module/rcupdate/parameters/rcu_tasks_trace_lazy_ms  -1   r--r--r--
```

Stated conservatively: the measurement supports "`rcu_expedited` does not cover the flavor
kprobe removal waits on". Which knob fixes the residual is untested.

### Finding 4 — the rebuild cost inverts across platforms

`eventMonitor.Init` is 3.50s on amazon_6.12 but 7.16s on ubuntu_22.04 — the rebuild is
*cheaper* on the platform where teardown is 5x more expensive. Teardown and rebuild need
separate treatment and the reload floor is platform-specific.

### What this means for the 55-minute job

**Rank by absolute seconds saved, not by speedup ratio.** The 10.3x fix applies to the kernels
that barely have the problem; the 1.71x fix applies to the kernels that do. Per platform class,
with the 43 teardowns of a real cws_host run (§1):

| class | teardown today | + `rcu_expedited` | saving |
|---|---|---|---|
| 5.15-class (10.3x applies) | 43 x 7.11s = 306s (5.1 min) | 43 x 0.69s = 30s | **-4.6 min** |
| 6.12-class (only 1.71x) | 43 x 34.28s = 1474s (24.6 min) | 43 x 20.02s = 861s | **-10.2 min** |

So `rcu_expedited` is worth *more* on 6.12 than on 5.15 despite the far smaller ratio, and it
should be applied on **every** platform rather than scoped to old kernels.

Stacking the levers on a 6.12-class job:

| scenario | teardown total | vs today |
|---|---|---|
| today | 1474s (24.6 min) | — |
| + `rcu_expedited` | 861s | -10.2 min |
| + Angle 3 grouping (43 -> 20 switches) | 20 x 20.02s = 400s | -17.9 min |
| + RCU-Tasks residual solved | 20 x ~0.7s = 14s | **-24.3 min** |

The 1474s projection independently reproduces the 1511s measured from timestamps in §1 — a good
consistency check.

**The stage's critical path is entirely the 6.11+ jobs.** `ubuntu_26.04` (61 min) and
`amazon_6.12` (57 min) set the wall clock and are the only two that blew the timeout.
`rcu_expedited` alone takes 61 -> ~51 min, still bad; only the RCU-Tasks residual takes it to
~37 min. The hardest item is the only one that fixes the timeouts.

Angle 3 is *more* valuable relative to Angle 2 than before, not less: once teardown is cheap,
halving the number of reloads is what removes the remaining time.

### Open gap: the 7s -> 34s cliff is not localized

There are only **two** data points, so we do not know where the cliff sits. It is *not* the
6.11 fentry cutoff: both platforms already run kprobe mode (Finding 3), so the fentry fallback
cannot explain a 5x difference. Some other kernel-version effect between 5.15 and 6.12 owns it.
`amazon_2023` (6.1) could fall on either side, which changes how many of the 25 x86 jobs are in
the expensive class and therefore the whole fleet-level projection.

Cheap way to close it: extend the `cws_teardown` matrix to `amazon_5.4`, `amazon_2023`,
`fedora_38`, `ubuntu_25.10` — 2 VMs each, ~7 min per job, one pipeline. That bisects the cliff.

### Revised priority

1. **Bisect the cliff** — extend the teardown matrix. One pipeline, and every projection below
   depends on the answer.
2. **The RCU-Tasks residual on 6.11+** — needs a kernel cmdline experiment (the `rcu_tasks_*`
   knobs are read-only). Hardest, but the only lever that fixes the jobs that actually time out.
3. **`rcu_expedited` on all platforms** — -10.2 min on 6.12-class, -4.6 min on 5.15-class,
   sysfs-only, no infra change.
4. **Angle 3** — group tests by static config, 43 switches -> 20.
5. **Angle 2d** — the `reflect.DeepEqual`-over-func-fields bug in `testOpts.Equal`; cheap.
6. **Angle 5** — timeout headroom, unchanged.
7. **Angle 1** — oversubscription. Demoted: real, but not the cause of the dominant cost.

### Artifacts

Jobs `1923956066` (amazon baseline), `1923956068` (amazon rcu), `1923956069` (ubuntu baseline),
`1923956070` (ubuntu rcu). Analysis scripts: `analyze_phases.py` (phase aggregation + A/B
table) and `probe_mode.py` (fentry mode, probe counts, RCU knobs) — both parse `[phase]` lines
out of the testjson artifact.

## 11. How to reproduce the analysis

```bash
# job trace (gitlab.ddbuild.io is OAuth-gated; dda handles auth)
dda inv gitlab.print-job-trace 1916277389 > job.log

# per-test timings + interleaved log lines come from the testjson artifact,
# which has test2json `run`/`pass`/`output` events with timestamps.
# No dda task downloads artifacts; use the repo's own gitlab client:
PYTHONPATH=$PWD ~/.local/share/dda/venvs/legacy/bin/python - <<'PY'
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
job = get_gitlab_repo().jobs.get(1916277389)
open("artifacts.zip","wb").write(b"") or None
with open("artifacts.zip","wb") as f:
    job.artifacts(streamed=True, action=f.write)
PY
unzip -o artifacts.zip
tar xzf test/new-e2e/tests/testjson-x86_64-amazon_6.12-cws_host.tar.gz   # -> out.json
```

Key derivations from `out.json`:

- reload set = tests with an `output` event containing `Instantiating a new security module`
- teardown cost = `ts(that output) - ts(run event)` per reload test
- config map = parse `withStaticOpts(testOpts{...})` literals per `func Test...` block in
  `pkg/security/tests/*_test.go`, then walk the executed order counting signature changes

Raw data for this run is preserved in `.ci-speed-data/` (git-excluded via
`.git/info/exclude`).

## 12. RESULTS — `rcu_expedited` on the whole KMT fleet (pipeline 129082991, 2026-08-05)

Commit `158488b79dba` moves the knob into `micro-vm-init.sh`, so **every** KMT microVM gets it —
secagent and sysprobe alike — and turns `DD_CWS_PHASE_PROFILING=1` on for `cws_host` so the
per-teardown cost is readable on all 43 kernels rather than the 2 of §10.

Baseline is pipeline `128540308` (`809fc228c854`), the reference run of §1. **The two pipelines
have identical job topology** — 210 `kmt_run_*` jobs, same names, no extra teardown jobs — so VM
count and contention are the same and the comparison is apples to apples.
`Setting /sys/kernel/rcu_expedited to 1` is confirmed present in every job trace checked.

### The headline

| group | jobs | baseline | with `rcu_expedited` | saved |
|---|---|---|---|---|
| `cws_host` (secagent) | 43 | 26.9 job-h | 21.9 job-h | **-18.6 %** |
| other secagent (`cws_peds`/`docker`/`ad`/…) | 89 | 20.1 job-h | 16.3 job-h | **-19.1 %** |
| **sysprobe** | 78 | 43.6 job-h | 36.1 job-h | **-17.2 %** |
| **total KMT** | 210 | **90.6 job-h** | **74.3 job-h** | **-16.3 job-h / -18.0 %** |

One `cws_host` job (arm64 `ubuntu_26.04`) was still running at 42.7 min when this was compiled;
it can only move the total by a few tenths of an hour.

The **sysprobe row is the cleanest arm**: no code in this branch touches system-probe and it gets
no phase profiling, so its 17.2 % is pure `rcu_expedited`. Biggest single movers there:
`amazon_2023 no_usm` 42.0 -> 19.0 min, `fedora_37 no_usm` (arm) 41.6 -> 21.1, `amazon_6.12 no_usm`
59.2 -> 42.4. Conversely the `cws_host` numbers are *understated*, because those jobs pay the
profiling logging overhead that the baseline did not.

Wall clock on the critical path:

| job | baseline | now |
|---|---|---|
| `x64 [ubuntu_26.04, cws_host]` | 61.1 min | **47.9 min** |
| `x64 [amazon_6.12, cws_host]` | 57.3 min (timeout panic) | **44.0 min, green** |
| `arm64 [ubuntu_26.04, cws_host]` | 48.1 min | ~42.7 min |
| `arm64 [amazon_6.12, cws_host]` | 48.0 min | 37.1 min |
| longest `cws_host` anywhere | 61.1 min | **47.9 min** |

**The timeout cliff of §1 is gone.** `amazon_6.12` was the one job that actually blew
`-test.timeout=55m` and threw away its rerun budget; it now finishes the suite in 44 min and
passes. Angle 5 (timeout headroom) drops from "safety net, do now" to ordinary hygiene.

### Validation — the delta is 10x the main-to-main noise floor

The table above uses one baseline pipeline, so it could in principle be a slow-day artifact. It is
not. Note first that `128540308` has **`ref=main`**, so it was already a pipeline of pure main code
(`809fc228c854` is an ancestor of main) — not a branch run. Adding three more full-matrix main
pipelines from the same 24 hours:

| KMT group | `128540308` | `129100089` | `129077517` | `129024665` | main median | **branch** | delta |
|---|---|---|---|---|---|---|---|
| `cws_host` (43) | 26.87 | 26.98 | 26.90 | 26.02 | 26.89 | **21.86** | **-18.7 %** |
| other secagent (89) | 20.08 | 19.61 | 19.78 | 19.14 | 19.69 | **16.26** | **-17.5 %** |
| sysprobe (78) | 43.62 | 43.39 | 43.54 | 43.60 | 43.57 | **36.13** | **-17.1 %** |
| **all 210** | **90.57** | **89.97** | **90.22** | **88.75** | **90.10** | **74.25** | **-17.6 %** |

Four independent main pipelines span **88.75-90.57 job-hours — a 2.0 % band**. The branch sits
15.8 job-hours below the median, roughly **10x the entire main-to-main spread**. Per job the median
main-to-main spread is 1.3 min.

Per-job wall clock against the main median:

| job | main min/med/max | branch | vs med |
|---|---|---|---|
| `sysprobe_x64 [amazon_2023, no_usm]` | 40.2 / 41.6 / 42.0 | **19.0** | -54 % |
| `sysprobe_arm64 [fedora_37, no_usm]` | 40.9 / 41.3 / 42.2 | **21.1** | -49 % |
| `secagent_x64 [amazon_2023, cws_host]` | 43.6 / 43.8 / 44.3 | **25.2** | -43 % |
| `sysprobe_x64 [amazon_6.12, no_usm]` | 59.1 / 59.2 / 61.8 | **42.4** | -28 % |
| `secagent_x64 [amazon_6.12, cws_host]` | 57.2 / 57.4 / 58.1 | **44.0** | -23 % |
| `secagent_x64 [ubuntu_26.04, cws_host]` | 53.3 / 60.6 / 61.1 | **47.9** | -21 % |
| `secagent_x64 [debian_12, cws_host]` | 36.9 / 38.6 / 38.9 | **32.1** | -17 % |
| `secagent_x64 [ubuntu_22.04, cws_host]` | 38.6 / 41.4 / 42.2 | **34.6** | -16 % |
| `secagent_x64 [centos_7.9, cws_host]` | 24.0 / 24.8 / 26.4 | 24.4 | -2 % |

Distribution over all 210 jobs: median **-15 %**, Q1 -8 %, Q3 -21 %, best -55 %. Every branch value
above is below the main *minimum*, not just the median, except `centos_7.9` — which is exactly the
prediction of Finding 5, since 3.10 barely had the problem.

Five jobs came out slower, and all five are noise rather than regression: four are 3-minute
`cws_peds` jobs (+1.4 to +3.0 min, on jobs whose own main range is 2.6-7.0 min) and one is
`sysprobe_x64 [rocky_9.4, only_usm]` at +0.5 min on a 34-minute job.

### Finding 5 — the residual is a kernel-*config* difference, not a version cliff

§10 left the 7s -> 34s cliff unlocalized and assumed it was a kernel-version effect somewhere
between 5.15 and 6.12. With `Manager.Stop` now measured on 12 x86 kernels, that assumption is
**wrong**. Median `EBPFProbe.Close/Manager.Stop` per teardown, `rcu_expedited` on:

| kernel | tag | n | median | sum | share of job |
|---|---|---|---|---|---|
| 3.10 | centos_7.9 | 41 | 0.75s | 33s | 2 % |
| 6.1 (amzn) | amazon_2023 | 52 | 0.78s | 43s | 3 % |
| 5.10 | amazon_5.10 | 54 | 0.79s | 45s | 2 % |
| 6.2 | fedora_38 | 53 | 0.79s | 43s | 3 % |
| 5.15 | ubuntu_22.04 | 53 | 0.94s | 52s | 2 % |
| 4.14 | amazon_4.14 | 49 | 0.98s | 48s | 3 % |
| 5.4 | amazon_5.4 | 52 | 0.98s | 54s | 3 % |
| 4.18 | ubuntu_18.04 | 50 | 0.99s | 54s | 3 % |
| 6.8 | ubuntu_24.04 | 54 | 1.10s | 62s | 4 % |
| **6.17** | ubuntu_25.10 | 64 | **7.29s** | 489s | **21 %** |
| **6.1 (deb)** | debian_12 | 53 | **7.74s** | 434s | **22 %** |
| **6.12** | amazon_6.12 | 53 | **19.83s** | 1111s | **42 %** |

Two facts kill the version hypothesis:

- **6.1 lands on both sides.** `amazon_2023` (6.1.119-amzn2023) is 0.78s; `debian_12`
  (6.1.0-39-amd64) is 7.74s. Same upstream minor, 10x apart.
- **6.8 is cheap** (1.10s) while 6.12 and 6.17 are not.

So the expensive set is 3 of 12 kernels and membership tracks how the *distro* configured RCU, not
the version. Candidate knobs (**untested**): preemption model / `CONFIG_PREEMPT_RCU`,
`CONFIG_TASKS_TRACE_RCU` lazy variants, `rcu_nocbs`/`nohz_full`. Comparing
`/boot/config-*` between `amazon_2023` and `debian_12` is the cheapest next step and needs no
pipeline. This also means the §10 fleet projection was pessimistic: **9 of 12 x86 kernels are
fully fixed by the sysfs knob alone**, and only 3 need the boot-param work.

### Finding 6 — the §10 per-teardown numbers reproduce across pipelines

Implied baseline per teardown, from wall-clock delta / teardown count, against §10's direct A/B:

| platform | implied from wall clock | directly measured (§10) |
|---|---|---|
| amazon_6.12 | 35.0s | 34.28s |
| ubuntu_22.04 | 8.8s | 7.11s |

And the post-fix medians match too (`amazon_6.12` 19.83s here vs 20.02s in §10; `ubuntu_22.04`
0.94s vs 0.69s). Two independent pipelines, two measurement methods, same answer.

### Test outcomes — 2 recoveries, 1 new failure

Only four jobs changed status out of 210:

| job | baseline | now |
|---|---|---|
| `x64 [amazon_6.12, cws_host]` | failed (timeout) | **success** |
| `x64_ad [ubuntu_22.04, cws_ad]` | failed | **success** |
| `arm64 [ubuntu_22.04, cws_host]` | success | **failed** |
| `arm64 [ubuntu_26.04, cws_host]` | failed | still running |

The pre-existing breakage is unchanged, which is the important negative result:
`x64 [ubuntu_26.04]` fails 62 tests now vs 63 before with the **same failing test set**
(`TestFlowPid*`, `TestNetDevice`, `TestTCFilters`, `TestMountEvent`, `TestMultipleProtocols*`,
`TestDNSResponse`, `TestProcessContext`), and `arm64 [ubuntu_25.10]` fails 13 with an identical
set. `rcu_expedited` did not perturb them.

The one regression is `arm64 [ubuntu_22.04, cws_host]`:
`TestFilterOpenLeafDiscarderActivityDump` fails 3/3 attempts with
`ContainerID ... not found on activity dump list ([])`, where the baseline job was completely
clean. That same test already fails on `arm64 [ubuntu_25.10]` and `x64 [ubuntu_26.04]` in **both**
pipelines, so it is a fragile activity-dump test rather than something new — but it went from
green to red on this platform and it is not in `flakes.yaml`. **A repeat pipeline is needed to
tell a flake from a timing regression**; expediting RCU does change teardown timing, and an
activity-dump test that races a dump period is exactly the shape that could notice.

### Correction to §10

§10 said `ubuntu_26.04` and `amazon_6.12` "are the only two that blew the timeout". Only
`amazon_6.12` did. `ubuntu_26.04`'s 61 min came from 63 genuine test failures and their reruns
(the suite itself finished in 1904s), which is why its wall clock improved 61.1 -> 47.9 min but it
is still red.

### Revised priority after this run

1. **Diff `/boot/config-*` between `amazon_2023` (0.78s) and `debian_12` (7.74s)** — same 6.1,
   10x apart. Free, no pipeline, and it names the boot parameter for item 2.
2. **The residual on the 3 expensive kernels** (`amazon_6.12` 19.8s, `debian_12` 7.7s,
   `ubuntu_25.10` 7.3s). Now scoped to 3 of 12 x86 kernels instead of "everything past 6.11".
   `amazon_6.12` is still 42 % teardown.
3. **Repeat the pipeline** to settle `TestFilterOpenLeafDiscarderActivityDump` on arm64
   `ubuntu_22.04`, and land `rcu_expedited` if it is a flake.
4. **Angle 3** (group tests by static config, 43 switches -> 20) — unchanged in value on the 3
   expensive kernels, now nearly worthless on the other 9.
5. **Angle 2d** (`testOpts.Equal` over func fields) — cheap, still valid.
6. **Angle 1** (oversubscription) and **Angle 5** (timeout headroom) — both demoted further.

### Artifacts

Pipeline `129082991`. Phase data pulled from the 12 x64 `cws_host` jobs
`1924166812/814/815/816/818/819/820/821/822/824/828/829` with `.ci-speed-data/analyze_phases.py`.
