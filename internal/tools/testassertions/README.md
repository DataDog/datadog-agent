# testassertions

A best-effort static analyzer that lists the assertions a Go test performs,
built for new-e2e tests but it works on any Go test. It only parses the code
(`go/ast`, no type-checking), so it is fast and ignores build tags: every
platform variant of a package is analyzed.

```bash
bazel build //internal/tools/testassertions
B=bazel-bin/internal/tools/testassertions/testassertions_/testassertions

$B -list test/new-e2e/tests/agent-subcommands                       # tests, suites, assertion counts
$B test/new-e2e/tests/agent-subcommands TestDefaultInstallUnhealthy  # a suite method
$B test/new-e2e/tests/agent-subcommands linuxStatusSuite.TestStatusHostname
$B test/new-e2e/tests/agent-subcommands TestLinuxStatusSuite         # a top-level test: expands the suite
$B -brief -run 'Redis' test/new-e2e/tests/discovery                  # summaries only
$B -json test/new-e2e/tests/discovery TestConfigDiscoverySuite       # machine readable
$B -max-bytes 32000 test/new-e2e/tests/fleet TestFleetConfigMacOS    # compact, fits a size budget
```

`bazel run //internal/tools/testassertions -- <dir> <test>` also works; relative paths
are resolved against the directory you ran it from.

## What it recognizes

- testify `assert.*` / `require.*`, suite methods (`s.Equal`, `s.NoError`, …),
  `s.Require().*`, `s.Assert().*`, `assert.New(t)` / `require.New(t)` objects,
  `t.Error*` / `t.Fatal*` / `t.Fail*` (also on `*assert.CollectT`)
- polling blocks: `Eventually`, `EventuallyWithT`, `Never`, `Condition` (also the
  `e2e.BaseSuite` wrappers); for `func() bool` conditions the `return` expressions
  are reported as conditions
- `e2e.Run` / `suite.Run`: the suite type is resolved (composite literal, variable,
  constructor return type, including constructors declared to return `e2e.Suite`
  and passed to a runner like fleet's `suite.Run(t, newConfigSuite, platforms)`,
  generic helper, or picked from a table in a loop: `e2e.Run(t, tc.suite)`,
  `for _, s := range suites`, `suite := test.t(...)` with `t` a constructor field;
  all the possible suites are expanded under "runs one of N suites") and its
  `Test*` methods and hooks
  (`SetupSuite`, `BeforeTest`, …) are expanded, including promoted methods
- `t.Run` / `s.Run` subtests, `if`/`else`, loops and `switch` cases, so conditional
  assertions are visible; `if assert.X(...) {` is labeled "if the assertion above passed"
- helpers, methods (through struct embedding, and on any expression of known type,
  e.g. `T{...}.Assert(t)` or fluent chains like `s.Require().Host(h).HasX()`),
  closures, function-valued parameters and function-valued struct fields
  (`e.test(p)` with `e := T{test: func(...) bool {...}}`) are followed recursively
  (`-depth`, default 8)
- predicates: a `func(...) bool` literal called in an `if` condition decides the
  branch (typically "return success when a payload matches"), so its `return`
  expressions are reported as conditions under `↳ predicate ...`
- `Must*` calls (e.g. `RemoteHost.MustExecute`) are reported as implicit checks `[M]`
- `t.Skip*` / `s.T().Skip*` (`⤼`) and `flake.Mark*` (`~`) calls, which tell when the
  surrounding assertions are not enforced
- `e2e.BaseSuite.EventuallyWithExponentialBackoff`: non-nil `return`s of the polled
  `func() error` are reported as failing the attempt

For each assertion it prints a human readable summary, the original call, the
failure message and, best effort, where the variables come from
(`err ← v.Env().Agent.Client.Health()`, `logFileName (package-level) = "hello-world.log"`).

It also lists the **values** each check is about (`· values: "gpu.sm_active", "gpu_uuid"`):
string literals traced through assignments, range loops, struct fields
(`sec.shouldContain` only yields that field of the struct literal), helper
parameters (back to the calling test) and constants, including those in imported
packages. Values are shown in `-brief` mode too. In `-json`, each test also carries
`values`, the union of all values checked below it, and `assertionCount`. For a
top-level test that runs suites, both include all the suite methods and hooks, so
an entry point describes everything that runs under it. This is handy to match a
change (a metric name, config key, command...) against the tests that check it.

A helper called several times in the same top-level test is expanded once;
later calls are shown as `↺ helper(...) — same checks as the call expanded @pos
(N assertion(s))`, with their own values and the function literals they pass
still expanded. Counts (`-list`, the footer, `assertionCount`) include those repeats.

## Compact output and size budget

`-compact` prints one line per node with ASCII indentation and values inline
(`[R] err is nil (no error) @config_test.go:47 {"merge-patch", "/datadog.yaml"}`);
collapsed repeats without values of their own are omitted (they are still
counted). It is meant for tools and LLMs.

`-max-bytes N` (implies `-compact`) keeps the output under N bytes: subtrees are
folded into their parent line one at a time, deepest first (largest first at the
same depth), until it fits. A folded line keeps the number of assertions below it
(`+12 below`) and the values found there, so the most useful information for
matching a change survives. Every E2E entry point fits in 32 KB this way.

Markers: `[R]` fatal (require/Fatal), `[A]` non-fatal, `⟳` (`~` in compact) polling block,
`↺` repeated helper call (collapsed),
`[M]` implicit `Must*` check, `[?]` a call that looks like an assertion helper
(`assert*`, `check*`, `verify*`, …) but could not be followed.

## Limitations

- No type information: method calls on values whose type cannot be inferred from
  the source (e.g. `s.Env().FakeIntake.Client().X()`) are not followed.
- By default only helpers from the test's Go module are followed (`-follow=module`).
  Use `-follow=repo` to follow helpers in other modules of the repository (for
  example `pkg/ssi/testutils`) or `-follow=package` to stay in the test package.
- Origins and values are computed from the assignments seen in source order,
  ignoring block scoping and control flow; values of a collapsed helper's inner
  checks are those of its first call.
