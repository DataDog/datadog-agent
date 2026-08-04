# Making `kmt_run_secagent_tests_*` (`cws_host`) faster

Investigation notes, 2026-08-04. Branch `daniel.mercier/faster-ci`.

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

## 9. How to reproduce the analysis

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
