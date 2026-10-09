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
  constructor return type, generic helper) and its `Test*` methods and hooks
  (`SetupSuite`, `BeforeTest`, …) are expanded, including promoted methods
- `t.Run` / `s.Run` subtests, `if`/`else`, loops and `switch` cases, so conditional
  assertions are visible; `if assert.X(...) {` is labeled "if the assertion above passed"
- helpers, methods (through struct embedding), closures, and function-valued
  parameters are followed recursively (`-depth`, default 8)
- `Must*` calls (e.g. `RemoteHost.MustExecute`) are reported as implicit checks `[M]`
- `t.Skip*` / `s.T().Skip*` (`⤼`) and `flake.Mark*` (`~`) calls, which tell when the
  surrounding assertions are not enforced
- `e2e.BaseSuite.EventuallyWithExponentialBackoff`: non-nil `return`s of the polled
  `func() error` are reported as failing the attempt

For each assertion it prints a human readable summary, the original call, the
failure message and, best effort, where the variables come from
(`err ← v.Env().Agent.Client.Health()`, `logFileName (package-level) = "hello-world.log"`).

Markers: `[R]` fatal (require/Fatal), `[A]` non-fatal, `⟳` polling block,
`[M]` implicit `Must*` check, `[?]` a call that looks like an assertion helper
(`assert*`, `check*`, `verify*`, …) but could not be followed.

## Limitations

- No type information: method calls on values whose type cannot be inferred from
  the source (e.g. `s.Env().FakeIntake.Client().X()`) are not followed.
- By default only helpers from the test's Go module are followed (`-follow=module`).
  Use `-follow=repo` to follow helpers in other modules of the repository (for
  example `pkg/ssi/testutils`) or `-follow=package` to stay in the test package.
- Origins are computed from the last assignment seen in source order, ignoring
  block scoping and control flow.
