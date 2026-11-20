# Writing an integration test

An integration test runs the components of a pipeline together, in-process, so that it catches the pieces disagreeing with each other, which unit tests on each piece miss. Its value comes from what it keeps real. Each section below starts with the rule, then why it matters.

Worked examples on `main`: `comp/healthplatform/invalidconfig_integration_test.go` and `comp/healthplatform/missedbytes_integration_test.go`. [#60753](https://github.com/DataDog/datadog-agent/pull/60753) adds `comp/healthplatform/invalidconfig_payload_integration_test.go`, which applies every pattern below; read it from that PR until it reaches `main` (it merged into the branch of #60252).

## Placement and naming

Put the test in the package that assembles the pipeline, usually the bundle's root package (`comp/<area>/`) or the package of the last hop, in a `<behavior>_integration_test.go` file with the package's build tags (often `//go:build test`). The name tells readers it is slower than the unit tests around it, and the location gives it access to the bundle's `Bundle()`.

## Wire the real components

Build the graph with `fxutil.Test[T](t, ...)`, using the real `fx/` module (or `Bundle()`) of every hop the behavior flows through. Mock only dependencies outside the behavior's boundary, such as hostname, workloadmeta or telemetry, and be able to say why each one is outside.

Why: a mock in the middle of the pipeline replaces exactly the code whose agreement the test exists to check. When a hop has no real module that runs in a test, say so in the report rather than mocking it silently.

Ask for the components to assert on through an `fx.In` struct:

```go
deps := fxutil.Test[struct {
	fx.In
	Store  storedef.Component
	Schema schema.Component
}](t,
	Bundle(),
	config.Module(),
	fx.Supply(config.NewAgentParams(datadogYAML)),
	schemafx.Module(),
	hostnameinterface.MockModule(), // the hostname is not part of the behavior
	// ...
)
```

## Keep a real local dependency real

When the audit names a local dependency as the boundary, a daemon (Docker, containerd), a kernel feature or driver, or hardware such as a GPU, the test runs against the real one rather than a fake of it. Gate it the way the repository does: a build tag for code that only builds with the dependency (`//go:build docker`), and a helper that skips with the dependency's name when it is absent at runtime (`pkg/gpu/testutil.RequireGPU`). Look for an existing helper before writing one.

`dda inv test` runs `--config=dd-agent-go-tests-only`, whose tag filter leaves out the test targets built for such dependencies (`tagset_bpf`, `tagset_bpf_nvml`, `tagset_bpf_functionaltests`, `tagset_functionaltests`, `tagset_npm`). Find the test's target and tags with `bazel query --output=build 'tests(//<package>:all)'`. When the target carries one of those tags, run that label alone with `bazel test`, in both Step 5 runs, the contributor's and the copy's, rather than `dda inv test` on the package: a recursive package target with the tag filter lifted would also build the package's other excluded variants, and an unrelated one can fail the run.

```bash
bazel test //<package>:<target> --test_tag_filters= --test_output=all --nocache_test_results --test_arg=-test.v --test_arg='-test.run=^TestX$/^<case>$'
```

`--test_tag_filters=` clears the repository's default filter (`bazel/configs/go_tests.bazelrc`), which excludes the same tags even for a label named on the command line: without it, the target builds and its test doesn't run.

A skip passes, so a run on a machine without the dependency proves nothing: Step 5 must show the subtests as `--- PASS`, not `--- SKIP`. Run them where the dependency is (a machine with Docker, or the CI job whose image provides it, found by its build tag in `.gitlab/`). When no such run is possible, report the test as written but **not run**, naming the dependency, rather than validated.

## Feed the input the way production does

When the input is configuration, write real files to `t.TempDir()` and load them with the real config components (`config.Module()` with `config.NewAgentParams(path)`, `sysprobeconfigfx.Module()`), not `config.NewMockFromYAML`. The config mocks skip the post-init fixups and allow a dynamic schema, so they don't load configuration like the Agent does.

The config components load into the global configurations. Reset them to the state the Agent starts with, and restore them at the end of the test so tests don't leak into each other:

- If `grep -n "func InitAgentConfigs" pkg/config/mock/*.go` finds it (added by #60753), call `configmock.InitAgentConfigs(t)`. It also sets the mock guards, so `configmock.New` returns the same configurations.
- Otherwise, do what it does in the test: save `pkgconfigsetup.GlobalConfigBuilder()` and `GlobalSystemProbeConfigBuilder()`, call `pkgconfigsetup.InitConfigObjects()`, and restore them with `SetDatadog` and `SetSystemProbe` in `t.Cleanup`. `InitConfigObjects` registers its override functions again on every call, so restore those too, or they pile up across tests. `comp/` may not import `pkg/config/setup` (depguard): prefer adding the helper to `pkg/config/mock` over a lint exception.

Environment variables are read while the configuration is built, so set them first, after clearing the `DD_*` variables of the process: a CI runner's `DD_API_KEY` or `DD_SITE` would otherwise change the result. Use `t.Setenv`, which restores them.

Other inputs follow the same rule: a real socket or listener for network input, a real file for a tailed log, a real check instance for a check.

## Capture the output at the observable boundary

For anything sent to Datadog, point the forwarder at an in-process fakeintake and read it back with its client:

```go
ready := make(chan bool, 1)
fi := fakeintakeserver.NewServer(fakeintakeserver.WithAddress("127.0.0.1:0"), fakeintakeserver.WithReadyChannel(ready))
fi.Start()
require.True(t, <-ready)
t.Cleanup(func() { _ = fi.Stop() })
client := fakeintakeclient.NewClient(fi.URL())
```

Port `0` avoids collisions between tests running in parallel. Shorten the sending interval in the configuration (to around 100ms) so the test waits milliseconds, not minutes.

For status, diagnose or flare output, call the component's provider and assert on what it returns.

## Assert on what the contract covers

Assert on all of the output the behavior's contract covers, and only on it. Why: asserting on a few fields lets the others regress silently, which is how a pipeline breaks without any test failing; asserting on fields outside the contract makes the test fail on unrelated changes.

- **The whole payload is the contract** (a health report, an issue's content): compare it against a golden file in `testdata/`, after replacing the volatile values (timestamps, hostname, version, temporary paths, hashed IDs) with placeholders. Keep the scenario inputs next to it, one set of files per scenario, and run one subtest per scenario. A golden file also documents the payload for reviewers, and a scenario is added by adding files.
- **One part of a shared payload is the contract** (a metric in the agent telemetry payload, which carries every other component's metrics too): extract that part wherever the payload puts it, and compare all of it, every series with its value and tags, rather than looking up the one series you expect. That is how an extra or missing tag, or a series that should not be there, fails the test.

When normalizing, mind the platform: a Windows temporary path appears JSON-escaped in the payload (`C:\\Users\\...`), so replace the escaped form and convert separators, or the test only passes on Linux.

## Wait for events, never for time

Use `require.EventuallyWithT` with a timeout of a few seconds and a short interval, and assert inside it, so the failure message shows the last diff.

Don't assert that something doesn't happen by watching a quiet window (`require.Never` for one second): on a slow runner the output may not have been sent yet, and the test passes without checking anything. Make the absence observable instead, with a signal that the producers you check have completed:

- A canary proves completion only for the producer that emits it, in the same run. Add a canary input that this producer turns into one known output, wait for it, then check it is the only output of that producer. For example, an env var set to an invalid value makes the `datadog.yaml` check raise one known issue in the same run as every other `datadog.yaml` violation; once it reaches fakeintake, no other `datadog.yaml` issue may be in the report.
- Producers running independently (separate checks, goroutines) each need their own completion signal: a canary of their own, a status or counter they expose, or a synchronous source of what they report. In the example above, the system-probe check runs on its own, so the test asserts its absence on the violations the schema recorded while loading, which the check reports as they are.

When several payloads arrive, pick the latest one explicitly, and break ties in favor of the later arrival.

## Keep tests independent

Some state is process-wide and outside your control: metrics registered in the global telemetry registry when their package loads (`pkg/util/telemetry`), for example. Other tests reset that registry (the telemetry mock, the telemetry fx module when it stops), leaving those metrics in a registry nothing gathers, so a test reading them passes alone and fails after them. Use the component the Agent uses for that state (for the registry, `GetCompatComponent()` from `comp/core/telemetry/impl`, imported as `telemetryimpl`, not the telemetry fx module), and put the test in a package of its own: a separate test binary, where nothing resets the state first. Say why in the package documentation, so nobody adds a resetting test next to it.

When the output reports changes since its previous emission (agent telemetry reports counters as deltas), have each scenario build its own pipeline and take a first, baseline output before acting. The scenario then asserts on what it caused only, whatever ran before it in the process.

Restore every global you change (configurations, override functions, registries, environment variables) with `t.Cleanup`. Don't run these tests with `t.Parallel()` when they replace process-wide state. Check that the test also passes when run alone (`dda inv test --targets=./<package> --test-args='-test.run=^TestX$/^<case>$'`), not only after the other tests of the package.

## Say what's not covered

List the boundaries the in-process test doesn't preserve (a real secret backend binary, packaging, the service manager, the real host identity) in the report and the PR description. If one of them matters, keep a single new-e2e scenario for it rather than the whole suite: `write-e2e` can add it.
