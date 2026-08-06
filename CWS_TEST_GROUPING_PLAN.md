# Group CWS functional tests by module config

> Implementation plan for the test-grouping work. The measurements it references live in
> `CWS_KMT_CI_SPEED.md` in this directory — read §1, §10 and §12 there first.

## Context

The `kmt_run_secagent_tests_*` `cws_host` jobs run the whole `pkg/security` functional suite
serially in one process. On kernel 6.12 that job takes ~57 min and has been hitting the
`-test.timeout=55m` cliff, which panics and destroys the retry budget.

Measured on real CI runs (full data in `CWS_KMT_CI_SPEED.md`, produced by the `[phase]`
instrumentation already committed on that branch):

- 196 top-level tests execute; **43 of them rebuild the eBPF manager**, and rebuilds account for
  ~25 of the 55 minutes.
- Per rebuild: teardown (`EBPFProbe.Close/Manager.Stop`) is 34.28s on kernel 6.12 and 7.11s on
  5.15; the rebuild side (`eventMonitor.Init` + `Start`) adds 4.2s / 9.1s.
- A rebuild happens whenever a test's **static** opts differ from the currently-live module's.
  Only ~20 distinct static configs exist, and **173 of 252 test funcs (69%) use the default config**
  — but non-default tests are scattered through source order, so an isolated one costs two rebuilds
  (switch in, switch back). Go runs tests in file-then-declaration order, so today's ordering is
  accidental.
- Caveat on the config census: an earlier count derived by parsing `withStaticOpts` literals out of
  test bodies **undercounted** non-default tests, because config often arrives via a helper
  (`runHardlinkTests`, `truncatedParents`). The 173/79 split above accounts for those helpers. Treat
  "~20 distinct configs" as a lower bound until the registry reports the real number.

**Goal:** make same-config tests run consecutively so each config builds the module once, and make
that ordering *self-maintaining* — adding a test must require no manual ordering step.

## Scope and sequencing

Three PRs, in this order (decided with the user):

- **PR 1 — grouping** (this is the bulk of the work): declaration API, call-site migration,
  grouped execution, guardrail.
- **PR 2 — config comparability cleanup**: move the func-valued opts to `dynamicTestOpts` and
  hand-write `Equal`. Worth ~4 rebuilds and unblocks the last 4 configs.
- **PR 3 — reclassify the accidentally-static fields**: mostly pays off in `cws_ad`, not
  `cws_host`. See the final section.

PR 2 is *not* required for PR 1 to work. Only **4 of the ~20 configs** carry func fields
(`TestReplay` via `ruleMatchHandler`; `TestEventMonitor`, `TestEventMonitorNoEnvs`,
`TestTracerMemfd` via `preStartCallback`). Those 4 cannot be expressed as declared data, so they
stay on the call-site escape hatch and keep rebuilding until PR 2. The other 16 configs — ~192 of
the 196 tests — group normally.

Only start with PR 1. And don't open any PR unless the user asks you to do it.

Rebuild targets: **43 today -> ~24 after PR 1 -> ~20 after PR 2.**

**Read §12 of `CWS_KMT_CI_SPEED.md` before starting.** `rcu_expedited` has already landed on this
branch (`158488b79db`) and cut total KMT cost 18 % fleet-wide, which removed the timeout cliff and
most of the per-teardown cost. That changes what grouping is worth and which numbers to measure
against — see "What grouping is actually worth now" in the Verification section. It does **not**
change the rebuild *count*, which is what this work reduces.

## Design decisions already settled — do not relitigate

| decision | why |
|---|---|
| Config is **declared per test** next to the func, not parsed from source | Static parsing already produced wrong answers: config often reaches `newTestModule` through a helper (`runHardlinkTests`, `truncatedParents`), so the literal is not in the test body. |
| Declaration via `var _ = declare(TestFoo, testOpts{...})` | One line adjacent to the func, no `init()` block, no central list, compiler-checked. Passing the **function value** (not a name string) means renames cannot desync. |
| No codegen, no generated artifact, no comment DSL | Capability-equivalent to plain Go but needs a generator + checked-in file + CI freshness check. |
| Func-valued opts will move to `dynamicTestOpts` and **stay inline in the test body** (PR 2) | They are per-invocation hooks. Once they are not part of the static config they never need to be hashable, so there is no need to hoist callbacks into a name table. |
| `forceReload` becomes a **declared property**, not just a call-site flag | It determines whether a test can share a module, so the scheduler needs it. Call-site form is retained as enforcement. |
| ~~A fresh-module test must be **first after a rebuild**~~ — **wrong, see "PR 1 as built"** | Go runs the tests a `-test.run` pattern selects in source order and no pattern can change that, so there is no way to put a fresh test first. Rebuilds per config are `1 + k`, not `max(1, k)`. |
| Ordering is executed as **one `go test` pass per group** | Go's testing package cannot reorder `m.Run()`. Each pass pays exactly one module build, which is the floor anyway. |
| **Undeclared means "default config"** — no `declare` needed for the common case | Measured: **173 of 252 test funcs (69%) use the default config**, and ~174 of 258 `newTestModule` call sites need no change at all. So the final pass *is* the default-config group — a legitimate group with a known signature and exactly one module build — not a leftover bucket. Only the 31% with a non-default config get a `declare` line. |
| **No second entry point.** `newTestModule` consults the registry itself | With undeclared ⇒ default, a default test's existing `newTestModule(t, nil, ruleDefs)` call is already correct and untouched. A declared test drops its `withStaticOpts(...)` and the config comes from the registry. No `requireModule` wrapper needed. |

## Files

Shared, both platforms (`//go:build functionaltests`, no OS tag — safe home for the new API):
- `pkg/security/tests/testopts.go` — `testOpts`, `dynamicTestOpts`, `optFunc`, `withStaticOpts`,
  `withDynamicOpts`, `withForceReload`, `Equal` (`reflect.DeepEqual`, ~line 112)
- `pkg/security/tests/testdecl.go` — **added by PR 1**: the declaration registry, `configSignature`,
  `resolveStaticOpts`
- `pkg/security/tests/testruns.go` — **added by PR 1**: the partition, `-cws-list-groups` output,
  the rebuild guardrail
- `pkg/security/tests/module_tester.go` — `var testMod *testModule`

Linux implementation (the only platform with reuse logic):
- `pkg/security/tests/module_tester_linux.go` — `newTestModule`; reuse branches at ~696
  (ebpfLess) and ~715 (static-opts-equal); fresh build from ~746
- `pkg/security/tests/module_tester_windows.go` — `newTestModule` has **no reuse logic** (always
  builds fresh). The new API must compile here but is inert.

Entry point / flags:
- `pkg/security/tests/main_test.go` — `TestMain`, `flag.*` registration in `init()`

Runner:
- `test/new-e2e/system-probe/test-runner/main.go` — `testPass()` (~236), suite loop (~279),
  `buildCommandArgs()` (~129, already builds `-test.run` from `run-only`), `concatenateJsons()`
- `test/new-e2e/system-probe/test-runner/xml.go` — `openAndDecode`/`decode` for junit merging

## Step 1 — declaration API

In `testopts.go`:

- `declare(fn any, opts testOpts, mods ...declOption) struct{}` — resolve the test name via
  `runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()`, trim the package qualifier, store in a
  package map. **Panic on duplicate registration** (declarations are scattered per file, so a
  silent last-wins would be a trap).
- `needsFreshModule()` — `declOption` marking a test as requiring a freshly built module. Preferred
  over `forceReload` as a name: it states the requirement, not the mechanism.
- ~~`variants{"map": testOpts{...}, "erpc": testOpts{...}}`~~ — **not built, see "PR 1 as built"**:
  under one `-test.run` per pass a subtest-level declaration cannot pay off. Lookup still resolves
  `TestX/sub` then falls back to `TestX`, so the mechanism is there if a use for it appears.
- `configSignature(testOpts) string` — stable equality-class key for grouping.
  **Invariant: `Equal(a, b) == (signature(a) == signature(b))`.** Add a table-driven test asserting
  this; if the two ever disagree, grouping silently stops working. Note that while `Equal` is still
  `reflect.DeepEqual` (until PR 2), any config containing a non-nil func violates the invariant —
  so `declare` must **reject func-bearing configs with a clear panic message** pointing at the
  escape hatch, rather than accepting something it cannot group correctly.
- `declareRuntimeConfig(fn any)` — **built as `declareUngrouped(fn, reason string, mods ...)`**, see
  "PR 1 as built". Marks a test that cannot share a module with any other, whether because its config
  is only known at run time (`enforcementExcludeBinary: which(t, "sleep")`,
  `activityDumpLocalStorageDirectory: t.TempDir()`), because it holds a callback, or because it
  deliberately builds several. It keeps using call-site `withStaticOpts`, but the declaration makes it
  **explicit and reviewable** that the test is scheduled apart from the declared groups. See Step 3.

**No `requireModule` wrapper.** `newTestModule` resolves static opts itself:

1. call-site `withStaticOpts` supplied -> use it (escape hatch);
2. otherwise a declaration exists for `t.Name()` -> use the declared config;
3. otherwise -> the default config, exactly as today.

Rule 3 is what leaves 173 default-config call sites completely untouched.

**Two guards, both cheap runtime checks — no AST analysis needed:**

- Call-site `withStaticOpts` supplied *and* the test has a `declare`d data config -> fail. The two
  disagreeing would cause a rebuild inside a group and silently break the guardrail.
- Call-site `withStaticOpts` supplied *and* the test has no declaration at all -> this is the
  "forgot to declare a non-default config" case, which would land the test in the default pass and
  cost two extra rebuilds there. Emit a counted warning during migration; promote to a hard failure
  once migration is complete. This is what makes "undeclared ⇒ default" an enforced invariant rather
  than an aspiration.

## Step 2 — migrate the non-default call sites

**Only the 31% that use a non-default config** — 79 test funcs, 84 `withStaticOpts` uses. The other
173 test funcs and ~174 call sites are already correct and are not touched.

Mechanical: hoist the literal into a `declare` line and delete the `withStaticOpts` argument.

```go
// before
func TestDNSResolver(t *testing.T) {
	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))

// after
var _ = declare(TestDNSResolver, testOpts{networkIngressEnabled: true})

func TestDNSResolver(t *testing.T) {
	test, err := newTestModule(t, nil, ruleDefs)
```

- The 5 helpers that call `newTestModule` on a test's behalf (`runHardlinkTests`,
  `truncatedParents`, `testFilterOpenParentDiscarder`, `testMountSnapshot`,
  `fetchRealisticEventSerializerInner`): for the two that take an `opts` parameter the literal
  already lives in the calling test, so it moves up a line and the parameter disappears. For the
  other three the helper owns a fixed config; move it to the callers or leave them on the escape
  hatch.
- **Leave on the escape hatch, deliberately:**
  - the 4 func-bearing configs (until PR 2);
  - `enforcementExcludeBinary: executable` (`TestActionKillExcludeBinary`), where
    `executable := which(t, "sleep")` is genuinely resolved at runtime;
  - configs referencing test-local values — `activityDumpLocalStorageDirectory: outputDir` where
    `outputDir := t.TempDir()`. These are
    unique per test by construction, so they always rebuild and grouping was never going to help
    them. Mostly activity-dump / security-profile tests, gated on `DEDICATED_ACTIVITY_DUMP_NODE`
    and largely skipped in `cws_host` — but see "Most opts are static by accident" below: this
    particular one is a misclassification, not a genuine exemption.
- **Not an exemption, despite appearances:** `expectedFormats` is a *constant literal*
  (`[]string{"json","protobuf"}`, or `[]string{"profile"}` in 18 places in
  `security_profile_test.go`) that merely happens to be assigned to a local variable. Declare it
  inline verbatim.

### Most opts are static by accident

`genTestConfigs` (`module_tester.go`, ~line 748) renders a **YAML config template** from the static
opts, and that file is read once when `NewEventMonitor` is constructed. So a field is static
because it is baked into a config file — **not** because it changes the eBPF program set. Verdict:

| field | genuinely needs a rebuild? |
|---|---|
| `disableERPCDentryResolution`, `disableMapDentryResolution`, `networkIngressEnabled`, `networkRawPacketEnabled`, `ebpfLessEnabled`, `disableRuntimeSecurity` | yes — program/probe selection |
| `enableActivityDump` | plausibly yes — changes what the ADM starts |
| `activityDumpTracedEventTypes` | **unknown — check.** Event types can drive probe selectors (`GetSelectorsPerEventType`) |
| `activityDumpRateLimiter`, `Duration`, `CleanupPeriod`, `TracedCgroupsCount`, `CgroupDifferentiateArgs`, `TagRules`, `SyscallMonitorPeriod` | no — userspace policy |
| `activityDumpLocalStorage{Directory,Compression,Formats}` | no — output config |
| `securityProfile{Dir,MaxImageTags,WatchDir,NodeEvictionTimeout}` | no — output/policy config |

Do **not** try to reclassify these inside PR 1 (see the follow-up section for why and how). PR 1
should treat the misclassified ones as escape-hatch cases and leave a `TODO` referencing this
table, so the next change knows they are not genuine exemptions.
Migration is incremental: declarations are authoritative when present, and anything not declared
keeps working unchanged via the default pass described in Step 3.

## Step 3 — execute the grouping

1. `-cws-list-groups` flag registered in `main_test.go:init()`, handled in `TestMain` after
   `flag.Parse()` and before `m.Run()`: print one line per pass, then `os.Exit(0)`.
   Because every declaration is registered at `init()` time this needs **no test execution** — no
   discovery pass, no skip-and-record, no source parsing.
   ~~Within each group, order declared-fresh tests first.~~ Not possible: `-test.run` selects tests
   but never reorders them, so a fresh test costs its group an extra build wherever it lands.

2. `test-runner`: in `testPass()`, if the suite advertises the flag, run one gotestsum pass per
   group with that pattern; otherwise fall back to today's single pass. **The fallback must be
   robust — this must never make CI worse than the status quo.**
3. Merge junit + testjson across passes (reuse `concatenateJsons` and the `xml.go` helpers). Verify
   no duplicate entries and that the total test count matches a single-pass run.
   **Gotcha:** the artifact paths are derived from the package name only —
   `junitfilePrefix := strings.ReplaceAll(pkg, "/", "-")`, then `<prefix>.xml` / `<prefix>.json`.
   N passes over the same package would write the same two files and silently clobber all but the
   last, so the merged report would look like most tests vanished. Add a group index to the prefix
   (`pkg-security-g03.xml`). `concatenateJsons` globs the whole json dir, so distinct names are
   picked up automatically; `addProperties` must run per pass against its own file.
4. **There is no parallelism to preserve.** `testPass()` is a sequential `for` loop with a blocking
   `cmd.Run()` — no goroutines, no errgroup, no `-p` — and the CWS suite is strictly serial anyway
   because the eBPF probe is a process-global singleton. Chaining passes costs only fixed per-pass
   startup: **measured 0.061s** for binary load + runtime + package init + `TestMain`
   (`testsuite -test.list ZZZ_no_match`), against 5-24s per avoided rebuild. Conversely, groups are
   independently runnable, so they make cross-microVM sharding nearly free later.
5. Give each pass its own `-test.timeout` rather than one 55m budget for everything. Side benefit: a
   hung test then kills only its own pass instead of aborting every rerun.

### The default pass — where undeclared tests run

This is the part that makes incremental migration safe, and it must be built before any call site is
migrated.

A test binary **cannot enumerate its own tests** — the `[]InternalTest` slice is private to
`testing.M`. So the default pass is expressed as a **skip of everything declared** rather than as a
list of what is left:

```
group k (declared):     -test.run  '^(TestA|TestB|…)$'
runtime-config tests:   -test.run  '^(TestC|TestD)$'      # each rebuilds anyway
default pass (last):    -test.skip '^(all declared + all runtime-declared names)$'
```

The default pass is a **real config group**, not a leftover: everything in it uses the default
config, so it pays exactly **one** module build for ~173 tests. That is the single biggest saving in
the whole design, and it costs no `declare` lines.

The `declareRuntimeConfig` tests cannot join it — their configs are non-default and unknown until
they run — so they get their own pass. Each forces a rebuild regardless, so grouping them together
versus separately costs the same; one shared pass is simpler.

`-test.skip` needs Go >= 1.21 (repo is on 1.26) and `test-runner` already emits it from the `Skip`
field of `packageRunConfiguration`, so the plumbing exists.

Properties this buys:

- **No enumeration, no drift.** A newly added default-config test is swept in automatically with no
  declaration and no regeneration — the self-discovery property being asked for, and the reason not
  to require `declare` for the 69% majority.
- **Strict non-regression.** Inside the default pass, tests run in normal declaration order. If
  nothing is declared yet, the pass *is* the whole suite and behaviour is byte-for-byte today's.
  That makes the migration safe to land in pieces.
- **Anchoring is sound for subtests.** `-test.run` splits the pattern on `/`, so `^TestA$` runs
  `TestA` and all of its subtests; `variants` are targeted as `^TestX$/^map$`.

Two guards:

- **Never empty by construction** — the default pass always holds the ~173 default tests, so unlike a
  leftover bucket it cannot degenerate to zero tests. Still, have the runner treat "0 tests ran" in a
  pass as success rather than failure, so a future matrix change can't break the job.
- **Every test in exactly one pass.** Assert in the runner that the declared patterns are pairwise
  disjoint and that the default pass's skip is exactly their union.

End state: once migration is complete, promote the "non-default opts without a declaration" warning
to a hard failure. Every non-default test then either `declare`s a data config or
`declareRuntimeConfig`s, the default pass holds exactly the default-config tests, and the guardrail
becomes exact.

## Step 4 — guardrail so it stays fixed

Expected rebuilds are computable from the declarations alone:

```
expected = Σ over declared configs C of max(1, freshCount(C))   # declared passes
         + count(declareRuntimeConfig tests)                    # each rebuilds
         + 1                                                    # the default pass
```

The committed `[phase]` instrumentation already logs every rebuild. Surface the observed count as a
KMT job tag (`kmt.tag-ci-job` already ships tags via `datadog-ci`).

Assert **per pass**, not on the total — that is where a regression hides:

- each declared pass: at most `1 + freshCount(C)` rebuilds;
- the **default pass: at most `1 + freshCount(default)`**, which is **6**, not 1 — five tests in it
  genuinely need a freshly built module (`TestMountPropagated`, `TestMountSnapshot{Listmount,Procfs}`,
  `TestSSHUserSession{Blocking,Snapshot}`). They rebuild today too, so this is not a cost grouping
  introduces, but it does mean the "default pass = exactly 1" assertion was never achievable.

Assert an **upper** bound, not equality: a skipped test builds nothing, and most of the
activity-dump tests are gated on `DEDICATED_ACTIVITY_DUMP_NODE`, so the observed count is routinely
below the expectation. Excess is the regression direction and the only one worth failing on.

A PR that adds a config or marks a test fresh changes the expectation automatically — no threshold to
bump, no baseline to regenerate.

## Risks

1. **Latent cross-test state coupling.** The suite has package-level globals (`testMod`,
   `commonCfgDir`). Reordering can expose dependencies that today's accidental order happens to
   satisfy. **Mitigation, do not skip:** land Step 3 behind the fallback, run grouped and ungrouped
   on the same commit, and diff the pass/fail sets before making grouped the default.
2. Multi-pass changes junit/testjson assembly and timeout semantics — verify artifacts are complete
   and test counts match a single-pass run.
3. `declare` name resolution depends on `runtime.FuncForPC` output format; assert the trimming in a
   test rather than trusting the string shape.

## Verification

Build and lint (never use raw `go` commands — build tags are computed by the tasks):

```bash
dda inv -e security-agent.build-functional-tests --output=/tmp/testsuite   # compiles + golangci-lint
```

Local, no VM needed:

```bash
/tmp/testsuite -cws-list-groups                    # one line per pass: declared groups, runtime-config, default
/tmp/testsuite -test.list '.*' | sort > /tmp/all   # cross-check coverage against the patterns
```

Coverage check to write as a test or a small script: every name in `/tmp/all` must match exactly one
declared `-test.run` pattern, or none of them (in which case the default pass's `-test.skip` must not
match it). This is the invariant that guarantees no test is dropped or run twice.

The `declare`/lookup tests and the `Equal`-vs-`signature` invariant test run as part of the suite
and need no eBPF.

End to end in CI: `cws_host` already has `DD_CWS_PHASE_PROFILING=1` enabled on
`daniel.mercier/faster-ci`, so a pipeline reports per-rebuild cost and rebuild count across all 25
x86 and 18 arm64 kernels. Baselines to compare against:

- rebuild count: **43** on `amazon_6.12`; expect **~24** after PR 1. This is the primary metric —
  it is unaffected by the teardown fixes, so it isolates what grouping did.
- **Use the post-`rcu_expedited` per-rebuild costs, not §10's.** `rcu_expedited` landed in
  `158488b79db`, so the branch baseline is now §12, not §10. Median `Manager.Stop` per teardown with
  the knob on: **19.83s** (`amazon_6.12`), **7.74s** (`debian_12`), **7.29s** (`ubuntu_25.10`), and
  **≤1.10s on the other 9 of 12 x86 kernels**. §10's 34.28s / 7.11s are pre-fix and must not be used
  as the comparison point.
- job wall clock on the branch: **44.0 min** (`amazon_6.12`, green), **47.9 min**
  (`ubuntu_26.04`, still red for unrelated pre-existing failures).

### What grouping is actually worth now

Because `rcu_expedited` already removed most of the teardown cost, grouping's payoff is smaller than
the pre-fix projection and **highly platform-dependent**. At current per-rebuild costs, removing ~19
rebuilds is worth roughly:

| platform | per rebuild (teardown + rebuild) | ~19 fewer rebuilds |
|---|---|---|
| `amazon_6.12` | ~24s | **~7.6 min** on a 44-min job |
| `debian_12` / `ubuntu_25.10` | ~12s | ~3.8 min |
| the other 9 x86 kernels | ~5-10s | ~1.5-3 min |

So grouping is still clearly worth doing on the slow kernels, but do **not** expect the ~15 min the
earlier projection implied — that number assumed a 34s teardown that no longer exists. Judge success
by the rebuild count dropping 43 -> ~24; treat the wall-clock saving as secondary and platform-specific.

Read results with `.ci-speed-data/analyze_phases.py <job_id>` from that branch, which pulls the
testjson artifact and aggregates `[phase]` lines. Job wall clock varies 29–61 min across identical
work, so judge by rebuild count and per-rebuild cost, not by a single job's duration.

## PR 1 as built

Landed. What differs from the plan above, and why.

### Deviations

| plan | as built | why |
|---|---|---|
| API in `testopts.go` | `pkg/security/tests/testdecl.go` (registry) and `testruns.go` (partition, guardrail) | ~450 lines; `testopts.go` keeps only the `staticOptsSet` flag that tells "no static opts" from "the default config, explicitly". |
| `declareRuntimeConfig(fn)` | `declareUngrouped(fn, reason, mods...)` | It has to cover three unrelated reasons a test cannot share a module — runtime-resolved value, callback, several modules on purpose — and the plan's name only describes the first. `reasonTempDir` / `reasonCallback` consts carry the shared ones plus their PR 2 / PR 3 TODOs. |
| `variants{...}` | not built; `buildsModules(n)` instead | See below. |
| fresh tests ordered first | `1 + freshCount` per pass | Go cannot reorder within a pass. |
| grouped execution on by default | opt-in via `CWS_TEST_GROUPING=1` | Risk 1 in this document asks for a grouped-vs-ungrouped diff on the same commit before flipping. The runner ignores the flag unless the env var is set. |

**Why `variants` cannot pay off.** `-test.run` splits its pattern on `/` and applies each element to
one level of the subtest path. A pass selecting `^(TestA|TestB)$/^map$` would apply `^map$` to
`TestA`'s and `TestB`'s subtests too, so a variant pattern cannot be joined to a group's alternation —
it needs a pass of its own, and a one-variant pass costs exactly the one build the variant already
costs inline. Zero saving for real machinery. Only two tests are genuinely multi-config
(`TestEventTruncatedParents`, `TestReplay` — not the 8 the plan assumed), so both are
`declareUngrouped` with `buildsModules(2)` / `buildsModules(4)`, which keeps the guardrail exact.

### Measured partition

From `testsuite -cws-list-groups`, cross-checked against `-test.list '.*'`: 258 tests, **every one in
exactly one pass**, none dropped, none duplicated.

| | passes | tests | expected rebuilds |
|---|---|---|---|
| declared config groups | 16 | 43 | 16 (exactly 1 each) |
| `ungrouped` | 1 | 29 | 33 |
| `default` | 1 | 186 | 6 |
| **total** | **18** | **258** | **55** |

The 16 configs — not the ~20 the plan estimated — are `networkIngressEnabled` (14 tests),
`networkRawPacketEnabled` (7), the two dentry-resolution configs (4 each), `dnsPort` and
`disableFilters` (2 each), and 9 single-test configs.

### Revised expectation: ~43 → ~33, not ~24

55 counts every pass; in `cws_host` most of the `ungrouped` run is the activity-dump and
security-profile tests, which are gated on `DEDICATED_ACTIVITY_DUMP_NODE` and skipped, and a skipped
test builds nothing. Expect roughly 16 (config groups) + ~11 (the ungrouped tests that do run) + 6
(default) ≈ **33**, against the 43 baseline — about **10 fewer rebuilds, not 19**.

The plan's ~24 assumed the default pass cost 1 rebuild (it costs 6) and that only 4 tests would stay
on the escape hatch (29 do). At the post-`rcu_expedited` ~24s per rebuild on `amazon_6.12` that is
**~4 min on a 44-min job**, and less on every other kernel — before subtracting the cost of 17 extra
test-binary startups, which is new and unmeasured. Measure it before assuming the trade is positive
on the fast kernels; consider restricting grouping to the slow ones if it is not.

### Guardrail

`-cws-group <name>` makes the suite count the modules it builds and exit non-zero if it exceeds that
group's prediction. The runner passes it automatically on every grouped pass, so a PR that puts a test
in a group whose config it does not use fails the job instead of silently losing the saving. The
check only fires upward: building fewer is normal.

`resolveStaticOpts` also warns, with a count and the test names, when a test passes `withStaticOpts`
without any declaration — the "forgot to declare" case. That is 0 today and should be promoted to a
hard failure once nothing legitimately needs the escape hatch (i.e. after PR 2 and PR 3).

## PR 2 — config comparability cleanup (deferred)

Worth ~4 rebuilds (~2.5 min on 6.12) and it lets the last 4 configs be declared. `Equal` is
`reflect.DeepEqual` over a struct containing func fields, and `DeepEqual` returns false for any
non-nil func — so a test setting one never matches, **and the next test doesn't either** (stored
non-nil vs new nil). Each func-using test therefore costs two rebuilds.

1. Move `preStartCallback` and `ruleMatchHandler` from `testOpts` to `dynamicTestOpts`.
2. Apply `ruleMatchHandler` on the **reuse** branches as well as the fresh path (today it is only
   applied at ~855, the fresh path). It already unregisters via
   `t.Cleanup(func(){ RegisterRuleEventHandler(nil) })`, so it is safe against a live module. This
   removes `TestReplay`'s forced rebuilds entirely.
3. `preStartCallback` genuinely needs a not-yet-started module — `TestEventMonitor` registers an
   event consumer between `Init` (~841) and `Start` (~864), and the reuse-path invocation at ~729
   runs against an already-started monitor. Give those 3 call sites an explicit
   `needsFreshModule()` declaration, preserving today's behaviour with the reason written down
   instead of emerging from a reflection quirk.
4. Keep `tagger` in `testOpts` — it feeds `emopts.ProbeOpts.Tagger` at construction (~787) and is
   genuinely not reconfigurable. Handle it explicitly in `Equal`: non-nil tagger on either side =>
   not equal. Do not rely on `DeepEqual` over an interface.
5. Replace `Equal` with a hand-written field comparison, then tighten the Step 4 guardrail to exact
   equality.

**Extra risk for PR 2:** step 2 makes `TestReplay` start *sharing* a module it has never shared.
That is the intended gain, but it is a behaviour change on a test with four `newTestModule` calls,
so it needs the same grouped-vs-ungrouped pass/fail diff as Step 3.

## Follow-up — reclassify the accidentally-static fields (PR 3)

Per the table in Step 2, most `activityDump*` / `securityProfile*` fields and several others
(`dnsPort`, the disarmer thresholds) do not affect the eBPF program set. Moving them from `testOpts`
to `dynamicTestOpts` shrinks the group count from below. The static/dynamic split is already exactly
the "rebuild-affecting vs reconfigurable" distinction, so this needs no new concept.

**Why this is deliberately not in PR 1:**

- The payoff is mostly in the **`cws_ad` job** (activity dump, ~21 min), not `cws_host`. In
  `cws_host` these tests are gated on `DEDICATED_ACTIVITY_DUMP_NODE` and skipped — only
  `TestFilterOpenLeafDiscarderActivityDump` benefits there.
- It is not a pure move. Because the values are rendered into a config file consumed at
  construction, making one dynamic means the reuse path has to push the new value into the **live**
  object and have the ADM pick it up. That is per-field work with per-field risk, unlike the
  mechanical migration in PR 1.
- For `activityDumpLocalStorageDirectory` there is a cleaner design than making it dynamic: let the
  **harness own the directory** and expose it (e.g. `test.activityDumpDir()`) instead of each test
  passing its own `t.TempDir()`. Then the field leaves `testOpts` altogether and those tests become
  ordinary declared tests. That is a refactor of the activity-dump tests, not of the opts plumbing.

Start by resolving the one **unknown** in the table: does `activityDumpTracedEventTypes` influence
probe selection via `GetSelectorsPerEventType` (`pkg/security/ebpf/probes/event_types.go`)? If it
does, it stays static and the rest still move.
