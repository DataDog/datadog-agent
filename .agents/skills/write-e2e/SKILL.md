---
name: write-e2e
description: Write or extend Datadog Agent new-e2e tests, including fakeintake coverage and the GitLab wiring that runs them; derives scope from the current diff when no target is named. Not for running tests that already exist (run-e2e, run-windows-e2e), or for judging whether a behavior belongs in E2E at all (e2e-audit).
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Skill, AskUserQuestion
argument-hint: "[behavior or feature to cover — omit to derive it from the current diff] [--area <tests-dir>] [--env host|docker|k8s|ecs] [--os linux|windows|both]"
---

Produce a mergeable E2E test for a change: correct scope, right environment, assertions that survive real infrastructure, and CI wiring that runs it.

**Ownership:** the team owning `test/new-e2e/tests/<area>/` is paged when the test fails, not `@DataDog/agent-devx`, which watches `main` and release branches and dispatches flakes back to that team. That is why the JOBOWNERS entry in Step 8 and the reliability gates in Step 9 are not optional.

## References

The body carries the procedure; the references carry the lookup tables. Load a file when its trigger fires.

| File | Load when |
|---|---|
| `references/environments.md` | The target is anything other than a Linux host, you need a non-default OS, architecture, or cloud, or you want a provisioner variant without the agent or without fakeintake. |
| `references/templates.md` | You need a skeleton other than the Step 5 one, you are splitting a suite across operating systems, or you need to mark a known flake or collect diagnostics on failure. |
| `references/fakeintake.md` | The test asserts on a payload — before writing the first such assertion, or when the payload you need has no client method yet. |
| `references/ci-wiring.md` | You are adding a new test package or changing which build artifacts a test consumes. |
| `references/api-traps.md` | You are working from an internal Confluence E2E page, an older branch, or any snippet whose form you cannot find in use under `test/new-e2e/tests/`. |

The repo is the source of truth; this skill is the procedure. Resolve an option by grepping its definition (`grep -n '^func With' <provisioner-dir>/*.go`) rather than recalling it.

## Step 0 — Establish scope

Given an argument, treat it as the scope and skip to Step 1. Otherwise derive it:

```bash
git status --short
git diff --name-only $(git merge-base HEAD origin/main)
```

Classify each changed path:

| Class | Examples | Action |
|---|---|---|
| User-visible behavior | a metric or log payload, a config key, CLI or service behavior, packaging, permissions | Candidate for coverage |
| Framework | `test/e2e-framework/**` | No test; validate by running an existing suite |
| Test-only, docs, CI | `test/new-e2e/tests/**`, `docs/**`, `.gitlab/**` | Out of scope |

Resolve the owning team and its `<area>` directory from `.github/CODEOWNERS`, then report the scope table before writing anything. When several behaviors are candidates and covering all of them would be a large change, ask which to cover. When nothing in the diff is user-visible, say so and stop — that is a correct answer, not a failure.

## Step 1 — Gate the scope

Invoke `e2e-audit` with the behavior. Continue to Step 2 only if the verdict is that E2E is justified; on any other verdict, report it, name the package where the test belongs instead, and stop. Gating before Step 2 is deliberate — reading implementations for the literals a test would assert on is wasted if the behavior belongs in a unit test.

Some areas own their own template and lifecycle rules through a directory-scoped skill: `ls test/new-e2e/tests/<area>/.claude/skills/*/SKILL.md`. If one exists, invoke it by its `name` and stop; directory-scoped skills are only offered when the working directory matches, so read the file directly when it is not in the skill list. Sibling skills named elsewhere in this file live under `.agents/skills/<name>/SKILL.md`.

## Step 2 — Extend an existing suite before creating one

Provisioning dominates E2E cost, and it is paid per suite. A Kubernetes suite waits out a ten-minute tagger warmup before its first assertion (twenty with FIPS enabled), and `tests/containers` already runs 25–30 minutes; a new suite next to it pays that again for one assertion. A new test method inside an existing suite is close to free.

Read the implementation now for the exact observable it produces, because the assertion depends on the literal string rather than on the feature name: a metric name, log service, check name, or tag if the behavior ships a payload; a service name, file path, registry key, package name, or CLI output if it does not. Then look for existing coverage and a suite to join:

```bash
ls test/new-e2e/tests/<area>/
grep -rn "<metric-or-config-name>" test/new-e2e/tests/<area>/
grep -rln "e2e.BaseSuite\[environments.<Env>\]" test/new-e2e/tests/<area>/
```

Widen the metric grep to all of `test/new-e2e/tests/` only when the area turns up nothing.

| Situation | Do |
|---|---|
| A suite in the area already provisions the environment you need | Add a test method to it |
| Same environment, but the suite needs a different agent config | Add a sibling entry point reusing the shared suite body |
| No suite in the area, or a genuinely different environment | Create one |

Read any `AGENTS.md` inside the target directory first (`ls test/new-e2e/tests/<area>/AGENTS.md`) — several areas carry local rules that override the defaults below.

## Step 3 — Choose the environment and cloud

| Signal in the behavior | Environment | Provisioner |
|---|---|---|
| Host metrics, agent CLI, files, services, packaging | `environments.Host` | `awshost.Provisioner()` |
| Windows host behavior | `environments.Host` with a Windows OS descriptor | `awshost.Provisioner()` |
| Active Directory, Defender, FIPS mode, test signing, MSI install | `environments.WindowsHost` | `winawshost.Provisioner()` |
| Agent in a container, Docker integrations | `environments.DockerHost` | `awsdocker.Provisioner()` |
| DaemonSet, Cluster Agent, admission, k8s tagging | `environments.Kubernetes` | kind (`.../aws/kubernetes/kindvm`) |
| ECS or Fargate task behavior | `environments.ECS` | `.../aws/ecs` |
| Two hosts, or a host plus a separate workload | custom struct | `e2e.WithPulumiProvisioner[Env]` |

Ordinary Windows behavior does not need `WindowsHost`. Suites like `tests/agent-subcommands/config_win_test.go` run on plain `environments.Host` with `ec2.WithOS(e2eos.WindowsServerDefault)`, which is also what the cross-OS split in `references/templates.md` produces — so a Windows test can extend an existing suite instead of provisioning a new one. Move up to `WindowsHost` only for the scenario components it carries.

Cloud: use the cloud the feature is specific to, and AWS otherwise. Windows included — the Windows CI templates depend on AWS-side MSI artifacts. Azure has a working Windows provisioner and reportedly boots Windows faster, but no test uses it and no CI job wires it, so choosing it means solving both problems yourself.

Kubernetes flavor: use kind. It provisions faster, costs less, and fails less than EKS. Reserve EKS for behavior that is specific to EKS, and expect an EKS job to run on `main` and nightly rather than on pull requests.

Every stock provisioner ships a fakeintake by default, and the intake is an ECS Fargate task you would otherwise pay for and never query. When the test asserts only on host state, drop it — but the way to do that differs by provisioner. Host provisioners (`awshost`, `winawshost`, and their Azure and GCP equivalents) offer a `ProvisionerNoFakeIntake` constructor; Docker, ECS, kind, EKS and kubeadm expose only `Provisioner`, so pass the scenario's `WithoutFakeIntake()` inside `WithRunOptions` instead.

Reach for a custom provisioner last. Check the option surface of the stock provisioner first — most needs are already an option, and custom provisioners are the main source of long-lived breakage.

## Step 4 — Place the files

Tests live in `test/new-e2e/tests/<area>/`. In an existing area, copy the package clause from a file already there — the usual form is the directory with its separators removed (`agent-runtimes` holds `package agentruntimes`), but not every area follows it. Read `test/new-e2e/AGENTS.md` §§ "Layout", "Each entry point needs its own suite type", and "Build tags" for the rest: which `<area>`, how a Linux/Windows pair splits across files, and why every entry point needs its own suite type — getting that last one wrong silently destroys infrastructure mid-run. `references/templates.md` § "Splitting one suite across operating systems" has the code for the split.

Fixtures: a YAML snippet of ten lines or fewer goes inline as a `const`; anything longer, and any script, goes in `fixtures/` loaded with `//go:embed` (which needs `_ "embed"` in the import block).

## Step 5 — Write the suite

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package myarea contains e2e tests for <feature>.
package myarea

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed fixtures/myfeature.yaml
var myFeatureConfig string

type myFeatureSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestMyFeature(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &myFeatureSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			// AWS agent options nest inside WithRunOptions. The flat
			// awshost.WithAgentOptions(...) reads naturally and is what
			// Azure and GCP use, but on AWS it does not exist.
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.Ubuntu2204)),
				ec2.WithAgentOptions(
					agentparams.WithIntegration("myfeature.d", myFeatureConfig),
				),
			),
		)),
	)
}

func (s *myFeatureSuite) TestMetricReachesIntake() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("myfeature.points")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics, "no myfeature.points received yet")
	}, 2*time.Minute, 10*time.Second)
}
```

The common `agentparams` options are `WithAgentConfig`, `WithIntegration`, `WithLogs`, `WithFile`, `WithTags`, and `WithHostname`; `references/environments.md` has the rest and the container equivalents.

Invariants, each with the reason it exists:

- Embed `e2e.BaseSuite[Env]` and start through `e2e.Run` — that is what builds `s.Env()` and registers teardown.
- Call `t.Parallel()` first in the entry point so suites overlap instead of serializing their provisioning.
- Keep sibling test methods independent. Testify does not guarantee ordering, so a method relying on a sibling's side effects fails unpredictably; put shared setup in the suite, and use `s.T().Run(...)` subtests when you genuinely need sequence.
- Leave the environment as you found it. Retries reuse the same infrastructure when they can, and fall back to a full reprovision when a leftover artifact breaks the second attempt.
- When overriding `SetupSuite`, `BeforeTest`, or `AfterTest`, call the embedded method first. Do not add `defer s.CleanupOnSetupFailure()` — `BaseSuite` already registers a `t.Cleanup` hook for it, and calling it from a `defer` consumes the panic so testify reports the recovered value with no stack, costing you the file and line of the failure. Older suites still carry the `defer`; leave them alone rather than copying them.
- Log what a future debugger needs on failure only. The infrastructure is destroyed after the run, and `E2E_DEV_MODE=true` is a local-only escape hatch. Write large dumps to `s.SessionOutputDir()` as an artifact rather than into the job log.

## Step 6 — Assert

Decide first what the behavior is observable as. Most suites in tree assert on host state — services, packaging, permissions, CLI output, resolved configuration — reached through `Env().RemoteHost`. Payload assertions through fakeintake are the minority; reach for them when the behavior *is* the payload.

Either way, anything that takes time to settle is asserted inside `s.EventuallyWithT(func(c *assert.CollectT) {...}, timeout, interval)` as in the Step 5 skeleton, and three rules apply to every such poll: `require` on anything later code dereferences, so the iteration aborts instead of accumulating misleading errors from a nil result; `MustExecuteOn(c, ...)` rather than `MustExecute`, so a transient SSH error retries; and never `time.Sleep` or a ticker, which is either a flake or wasted minutes.

**Asserting on payloads** — load `references/fakeintake.md` first. It has the payload-to-client-method table, the matcher catalog, the timeout budgets, how to prove a negative, and flush ordering around a restart.

**Not asserting on payloads** — drop the intake. On AWS it is an ECS Fargate task, so provisioning one the test never queries is pure cost against the budget Step 2 is trying to protect. How to drop it depends on the provisioner, as Step 3 notes.

## Step 7 — Keep it reliable

Read `test/new-e2e/codereview_guideline.md` now — it is the authoritative reliability contract, and three of its sections shape code you are about to type:

- § "Docker image pulls" — every image comes through the ECR cache. CI is moving to block outbound internet, so an unqualified `image:` or `FROM` resolves to DockerHub and will start failing.
- § "Pin your dependencies" — an unpinned image is `latest`, and `apt install <pkg>` silently tracks whatever the mirror carries today.
- § "Other dependencies / Internet accesses" — third-party artifacts come from the internal S3 bucket via `RemoteHost.HostArtifactClient`, and remote Kubernetes manifests are vendored locally with their image references rewritten, because a remote manifest pulls both itself and its images at runtime.

## Step 8 — Wire into CI

Needed when you created a new package, and whenever a test starts consuming a build artifact its job does not already pull — adding a container-based test to a package whose job extends a deb-only template leaves it waiting on an image nobody built. A test in a package no job references never runs at all. Load `references/ci-wiring.md` for the rule template, the artifact-dependency table, the Windows matrix convention, and which branches a job ends up running on. Two things bite regardless of which template you extend: overriding `needs` replaces the template's list rather than adding to it, so re-reference the template's own entry; and `.gitlab/JOBOWNERS` is what routes a failure notification, while `.github/CODEOWNERS` only routes review.

## Step 9 — Verify

Needs `dda`, `pulumi`, `~/.test_infra_config.yaml` (created by `dda inv e2e.setup`), sandbox AWS credentials, and AppGate connected; `run-e2e` covers setup failures. Run the gates in order and stop at the first failure.

| Gate | Command | Catches |
|---|---|---|
| G1 | `dda inv linter.go --targets=test/new-e2e/tests/<area>` | Compile errors, including nonexistent provisioner options |
| G2 | `grep -rn -e 'docker\.io' -e 'apt-get install' -e 'apt install' -e 'yum install' -e 'curl http' test/new-e2e/tests/<area>/` | External dependencies and unpinned installs |
| G3 | `grep -rn 'TARGETS:.*<area>' .gitlab/test/e2e/e2e.yml .gitlab/windows/test/` and the JOBOWNERS entry | A test that never runs, or fails silently to no owner |
| G4 | The dev-mode session below | Provisioning, timing, real Agent behavior, hidden inter-test dependencies, missing cleanup |
| G5 | Compare G4's wall time to 15 min (PR-gated) and 30–40 min (main/nightly) | A suite that will slow every pipeline |

G2 flags candidates for review, not violations. A parameterised registry such as `${DD_REGISTRY:-docker.io}` resolves to the cache in CI and is fine; a hardcoded `docker.io/...` or a package install on the VM is not. G3 is a grep, so it comes before G4 — there is no point proving a test works before knowing whether anything runs it.

G4 is the only gate that provisions real cloud infrastructure, and it takes 10–40 minutes. Dev mode keeps the stack alive between runs, so all three live checks share one provision:

```bash
# 1. One subtest on fresh infrastructure — proves it does not depend on a sibling.
E2E_DEV_MODE=true dda inv new-e2e-tests.run --targets=./tests/<area>/... --run '^TestX$/^SubB$'
# 2. The whole suite, reusing that stack.
E2E_DEV_MODE=true dda inv new-e2e-tests.run --targets=./tests/<area>/...
# 3. Again — a second green run proves each test left the environment as it found it.
E2E_DEV_MODE=true dda inv new-e2e-tests.run --targets=./tests/<area>/...
# 4. Destroy the stack. Without -s this removes only lock files and leaves it billing.
dda inv new-e2e-tests.clean -s
```

Show these commands and get agreement before running any of them, pairing the runs and the cleanup in one approval. Compilation alone is not evidence the test works, so treat skipping G4 as a gap to report rather than a shortcut.

## Output

```
Scope        behaviors covered, behaviors dropped and why, e2e-audit verdict
Files        created and modified
Environment  env struct, cloud, OS, and one line on why that combination
Assertions   observable -> how it is checked -> timeout, one row each
Local run    command, result, wall time
CI           the rule/job/JOBOWNERS diffs, or "none needed" and why
Follow-ups   flake exposure, budget headroom, QA label, PR-branch coverage
```

Filled in, for a diff that added a `disk.free` metric:

```
Scope        Covers disk.free reaching the intake. Dropped the config-parsing
             change — e2e-audit called it a unit test. Verdict: E2E justified.
Files        test/new-e2e/tests/agent-runtimes/disk_free_test.go (new)
Environment  environments.Host, AWS, Ubuntu2204 — host metric, no container
             or cluster behavior involved.
Assertions   disk.free -> FilterMetrics + WithMetricValueHigherThan(0) -> 2m/10s
Local run    dda inv new-e2e-tests.run --targets=./tests/agent-runtimes/...
             --run '^TestDiskFree$'  — passed, 11m4s
CI           None needed. new-e2e-agent-runtimes already covers ./tests/
             agent-runtimes and pulls agent_deb-x64-a7.
Follow-ups   Test-only change, so qa/no-code-change. Runs on PR branches
             via .on_arun_or_e2e_changes. 11m of the 15m budget used.
```

Recommend a QA label, and note that the checks accept exactly one. A pull request that only adds tests or CI wiring takes `qa/no-code-change`, whichever branches its job runs on. `qa/rc-required` is for changes that can only be validated on a release candidate — a workload that cannot be emulated, or behavior observable only during RC deployment — so reach for it based on what the *change* needs, not on whether the new job happens to skip pull-request branches. `docs/public/guidelines/contributing.md` has the full list.

When a repository document turns out to be wrong, correct it in the same change — the root `AGENTS.md` asks for exactly that.
