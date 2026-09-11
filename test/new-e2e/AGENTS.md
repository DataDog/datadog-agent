# E2E tests

Tests that provision real cloud infrastructure, install the Agent on it, and assert against the running system.

What they assert varies. Some check payloads arriving in the fakeintake; others check host state — Windows services, registry keys and ACLs (`tests/windows/`), package install, upgrade and rollback (`tests/installer/`, `tests/fleet/`), CLI output (`tests/agent-subcommands/`), resolved configuration on disk (`tests/agent-configuration/`). What makes a test E2E here is the real provisioned host and the really installed Agent, not the assertion target.

The framework these tests import is documented in `test/e2e-framework/AGENTS.md`, the intake mock in `test/fakeintake/AGENTS.md`.

`codereview_guideline.md` in this directory is the authoritative contract for making a test reliable and fast — read it before writing or reviewing one. This file covers how the tree itself is shaped, which is what a drive-by edit or a review needs to know.

## Layout

Tests live in `tests/<area>/`, where `<area>` is the owning team's directory in `.github/CODEOWNERS`. Area directories are hyphenated but Go package names cannot be, so a new area's package is usually that directory with its separators removed — `tests/agent-runtimes/` holds `package agentruntimes`. Existing areas do not all follow it; `tests/security-agent-functional/` declares `package secagentfunctional`. Read the package clause of a file already in the directory and match it exactly, since one directory cannot hold two package names.

One platform is one `<feature>_test.go`. A Linux and Windows pair shares a body: the suite struct and every assertion live in `<feature>_common_test.go`, while `<feature>_nix_test.go` and `<feature>_win_test.go` hold only entry points, each setting a different `descriptor e2eos.Descriptor`. Branch on `s.descriptor.Family() == e2eos.WindowsFamily` inside the shared body for path differences. `tests/agent-runtimes/infra_basic_*_test.go` is the reference implementation.

## Each entry point needs its own suite type

`BaseSuite` derives the default Pulumi stack name from the suite's struct type name — `e2e-<TypeName>-<hash(PkgPath)>`, in `test/e2e-framework/testing/e2e/suite.go`. Two entry points that instantiate the same type therefore land on the same stack, and since both run under `t.Parallel()`, two suites reconcile and tear down one shared stack concurrently. Nothing detects this: no compile error, no warning, just a confusing failure in whichever suite loses the race.

So `infra_basic_nix_test.go` declares `basicLinuxSuite` and `infra_basic_win_test.go` declares `basicWindowsSuite`, both embedding the shared `basicSuite`. Give every entry point its own named type, or pass a distinct `e2e.WithStackName`. The same applies to one test function that provisions per-platform stacks in a loop.

## Build tags

Ordinary e2e tests carry none. The `_nix`/`_win` filename suffix is a convention teams adopted for readability: it affects neither compilation nor which tests run. CI selects on the *test function* name, passing `--run` and `--skip` regexes through `EXTRA_PARAMS`, so name a Windows entry point such that a Linux job's `--skip "Windows"` excludes it — renaming the file would do nothing.

The exception is `e2eunit`. A package holding plain unit tests alongside e2e tests marks its e2e files `//go:build !e2eunit`, and the unit-test job in `.gitlab/build/source_test/linux.yml` runs with `--tags e2eunit`. Only `tests/installer/windows/` and `tests/windows/common/agent/` do this today; do not add the tag to a test with no unit-test sibling.

## Suite-owned fakeintake lifecycle

Most standard environments share one fakeintake across a suite, and the framework does not reset it between tests. Custom environments can intentionally use several receivers or several Agents; `tests/agent-runtimes/forwarder_nss_failover_test.go` is an example with two fakeintakes. One fakeintake is a common default, not a cardinality restriction.

A suite that needs each test to see only its own payloads resets the relevant suite-owned intakes itself, calling `FlushServerAndResetAggregators()` from `BeforeTest`. For ordinary tests, prefer that reset to adding artificial tags solely to filter out an earlier test's data. Tests deliberately comparing multiple senders or receivers must also assert the intended producer/destination identity. Do not flush an intake owned by another concurrent test or external workflow.

## Working in this tree

Use the `write-e2e` skill (`.agents/skills/write-e2e/`) to author or extend a test: it covers scoping from a diff, environment and cloud choice, fakeintake assertions, and GitLab wiring. `run-e2e` and `run-windows-e2e` cover running tests that already exist.

Several areas add local rules that override the defaults above, either in their own `AGENTS.md` (`tests/windows/`, `tests/gpu/`, `tests/installer/windows/`, and others) or in a directory-scoped skill under `tests/<area>/.claude/skills/`. Check for both before writing in an area.

## Keeping this file accurate

Part of the `AGENTS.md` hierarchy — see root `AGENTS.md` § "Keeping AI context accurate". Update it when the tree's layout conventions, suite-naming rules, or build-tag usage change. Reliability and speed rules belong in `codereview_guideline.md`; framework APIs belong in `test/e2e-framework/AGENTS.md`.
