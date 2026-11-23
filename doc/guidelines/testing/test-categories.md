# Test categories

Every test pays for the boundary it keeps real: a unit test pays milliseconds, an E2E test pays a cloud VM and tens of minutes. Put each assertion in the cheapest category that still catches the failure it exists for, and keep the expensive categories for failures nothing cheaper can catch. This page is the set of criteria for that choice. The <<<repo(".agents/skills/e2e-audit/SKILL.md", "`e2e-audit`")>>> and <<<repo(".agents/skills/write-test/SKILL.md", "`write-test`")>>> skills apply it, and reviewers can apply it by hand.

## Choosing a category

| The failure you need to catch | Category |
| --- | --- |
| Code gives a wrong answer for known inputs | [Unit](#unit) |
| Two of our components disagree, and everything outside them can be faked or run locally | [Integration](#integration) |
| A real local dependency, such as a daemon, driver, or hardware, behaves differently from our fake | [Integration](#integration), against the real dependency |
| The binary we built misbehaves as a program on one machine | [Binary test](#binary-test) |
| The failure appears only when the installed Agent runs in a real environment | [E2E](#e2e) |
| The failure appears only on some kernels | [Integration](#integration), on the [kernel axis](#environment-axes) |
| The failure appears only on some distros or package formats | [E2E](#e2e), on the [platform axis](#environment-axes) |

## How to decide

Analyze observable claims and failure modes, not test files or individual `assert` and `require` calls. Group the checks that validate the same contract into one assertion, including implicit claims such as a successful installation, startup, or command execution.

Do not ask only whether an assertion *can* be made with mocks. Ask what the assertion would stop validating if its dependencies were replaced. Two questions usually settle it:

1. **Which components must the test keep real?** List the hops the behavior flows through, from where its input enters to where its output is observed. When they are all our own code and can run in one process, the assertion is an integration test at most.
1. **Do those components depend on the platform?** Platform-specific code that builds and runs on a CI runner, such as a `_windows.go` file or a registry read, is still testable in a unit or integration test on that platform's unit test job. Only a dependency on the deployed system, such as the service manager, the installed package, its permissions, or the kernel, pushes the assertion higher.

When reviewing, ask three questions of each test:

1. What failure does this test catch?
1. What is the cheapest category that still catches it?
1. If the same claim is also tested in a more expensive category, what does that test catch that the cheaper one does not?

## Unit

**Owns:** a wrong answer from code whose inputs are known: parsers, aggregators, config validation, encoding, state machines, tag computation.

**Use when** the behavior is isolated logic that does not require a real component graph, process, or external dependency. Unit tests run in-process with `dda inv test`, in milliseconds, with no network or sleeps, and in any order.

**Does not catch:** wiring between components, the assembled binary, the kernel, the package, or the deployed configuration.

**Avoid:** mocking so much that the test restates the implementation, and kernel-independent tests that run once per kernel because they share a package with kernel tests.

## Integration

**Owns:** two of our components disagreeing through their real interfaces, or our code disagreeing with a real local dependency.

**Use when** the important boundary can be preserved locally: components wired for real with `fxutil.Test`, an HTTP client against a local server, configuration loaded from real files, a local fakeintake, or a real local daemon, driver, or hardware. A real local dependency does not by itself require E2E. Integration tests are still Go tests, and take seconds.

An integration test is only as good as what it keeps real:

- Use the real `fx` module of every hop the behavior flows through. Mock only dependencies outside the boundary, and be able to say why each one is outside. A mock on the very boundary the test exists to check makes the test pointless.
- Feed the input the way production does. Write configuration to real files and load it with the real config components rather than the config mocks, which skip the Agent's post-load fixups.
- Capture the output where it becomes observable: an in-process fakeintake for payloads, the component's provider for status, diagnose, or flare output.

`dda inv test` leaves out the test targets tagged for a local dependency (the `tagset_*` filters in <<<repo("bazel/configs/go_tests.bazelrc")>>>). Such a test runs in CI only in a job that selects its tag on a runner that provides the dependency.

The `write-test` skill's <<<repo(".agents/skills/write-test/references/integration.md", "integration reference")>>> has the wiring patterns, and <<<repo("comp/healthplatform/invalidconfig_integration_test.go")>>> is a worked example.

**Does not catch:** anything that only appears in a shipped binary or a running service, or kernel behavior that differs from the kernel the test runs on.

## Binary test

**Owns:** the binary we built misbehaving as a program, on one machine: its CLI parsing and output, startup and configuration merging, a subcommand talking to a running Agent, or two of our binaries talking to each other over local sockets and files.

**Use when** the assertion needs the built binaries running, but not the installed package, the service manager, a specific kernel, or a cloud API. These tests start the built binaries in a temporary directory with a local configuration and fakeintake, and take minutes.

**Status:** only the trace-agent has a binary test harness: <<<repo("cmd/trace-agent/test")>>> builds the `trace-agent` binary, runs it with a test configuration against a fake backend, and runs in CI as `trace_agent_integration_tests`. The other Agent binaries have no shared harness or CI job yet. Until they do, an assertion that belongs here stays in E2E, unless an integration test preserves the same failure.

**Does not catch:** the installed package, permissions it creates, service-manager behavior, other kernels, or cloud APIs.

**Avoid:** re-testing parser branches through the binary, and depending on paths that only the package creates.

## E2E

**Owns:** a failure that appears only when the installed Agent runs in a real environment.

**Use when** the deployed Agent or its real environment is part of the behavior being validated. This includes boundaries such as:

- installation, packaging, permissions, or merged deployed configuration;
- process or service lifecycle, cross-process communication, or CLI behavior that depends on the running Agent;
- kernel, operating-system, cloud, container-orchestrator, or network behavior that cannot be represented faithfully by a local dependency;
- user-visible data reaching fakeintake when the Agent's real assembly, configuration, encoding, or forwarding path is material to the test.

A behavior is not E2E-worthy merely because the current test reaches it through SSH, a CLI, or remote infrastructure. If those layers add no relevant coverage, use a cheaper category.

E2E tests live in <<<repo("test/new-e2e")>>>. Each suite provisions its own VM, cluster, or ECS environment with Pulumi, and a run takes 10 to 40 minutes. <<<repo("test/new-e2e/codereview_guideline.md")>>> has the reliability rules and time budgets.

**Does not catch:** every kernel, every distro, or every branch of a parser. Those belong lower or on an axis.

**Avoid:** asserting logic already covered lower down with nothing added, one suite per edge case, and tests that read another test's payloads from a shared fakeintake.

## Environment axes

Running the same tests on many kernels or distros does not make them a higher category; it multiplies the cost of the category they belong to. Put a test on an axis only when its result can change with that environment.

- **Kernel axis:** kernel matrix testing runs Go test packages in micro VMs, once per kernel. It owns failures that exist only on some kernels: a verifier rejection, a missing hook, a changed struct offset. A costly kernel-independent test in those packages belongs in the regular unit test jobs instead.
- **Platform axis:** E2E install suites on real distro images. They own failures that exist only for some distros or package formats: a missing systemd unit, wrong permissions, a broken upgrade path. Do not repeat product feature tests per distro when only the installation differs.

## Layered coverage

Testing a claim in two categories is useful when the cheaper test preserves its failure mode: it gives faster pull request feedback, more deterministic failures, and easier debugging. Do not treat that duplication as waste by itself.

After accounting for the cheaper coverage, reevaluate what the expensive test uniquely validates. Repeating an assertion through the deployed Agent can still be valuable when it catches assembly, configuration, packaging, lifecycle, or forwarding failures that the cheaper test cannot. If nearly all material assertions have equivalent cheaper coverage and the E2E test preserves no meaningful additional boundary, remove it: its feedback no longer justifies its provisioning, runtime, and maintenance cost. Base that decision on residual failure coverage, not on the number of duplicated assertions.

## Other checks

Some questions are not about behavior and belong to dedicated checks rather than any category above:

- **Static checks:** is this a valid program, schema, or generated file? Linters, generated-code checks, dependency diffs.
- **Artifact verification:** is the artifact we ship right before anyone runs it? Files inventory, binary and package size, static quality gates.
- **Performance:** did latency, CPU, memory, or throughput move? Benchmarks and the regression detector in <<<repo("test/regression")>>>.
- **Fuzzing:** does a parser accept or crash on input nobody wrote a case for?
