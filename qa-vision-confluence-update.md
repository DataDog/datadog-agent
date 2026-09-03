# Proposed update — "My vision regarding testing and qa experience on datadog-agent"

> **What this is:** a proposed new version of your Confluence page
> ([current page](https://datadoghq.atlassian.net/wiki/spaces/~7120201870126a495245b69e47156354de0ad9/pages/7146275864/My+vision+regarding+testing+and+qa+experience+on+datadog-agent)),
> based on the working session that sharpened the draft.
> Built against page version 4 (fetched 2026-09-01); the Atlassian OAuth session expired
> before this proposal was written — if the page changed since, reconcile before pasting.
>
> **How to use it:** review the changelog below, then paste everything after the
> `---` separator into the page.

## Changelog vs the current page

**Kept from your v4:** the disclaimer, the CLI-first framing, the local iteration workflow
(`start` / `list` / `install` / `update`), the environment-type breadth, the install-method
breadth, fakeintake inspection, the Kubernetes scenario narrative.

**Changed or added:**

1. **"Why" section added** — the evidence (integrations-core, ASU, agent-health requests)
   and the framing "CI works, local is the weak spot" now open the doc.
2. **"Vision" sharpened** — one inversion sentence carries the core idea: environments go
   from *ephemeral, owned by the test* to *durable, owned by the developer/agent*.
3. **"The configuration language" added** — one schema for everything; one environment =
   one definition (environment / agent / workloads sections); a **shared catalog of
   "classic" definitions** that developers start from and tests build on; and the key
   principle: **values when creating, constraints when declaring needs**.
4. **Two precise scenarios** replace the single scenario paragraph: a Kubernetes-feature
   developer and a core-agent-feature developer, same verbs, only the definition differs.
   Core-agent install is **binary-first (no .deb)**; workloads are deployed by the tooling
   and **wired to the agent** (dogstatsd/APM endpoints injected).
5. **"Running tests on an existing environment" added** — each test's `needs` (constraints)
   are checked against the live environment; fakeintake wiped per run (archivable first);
   one run per environment at a time; test mutations are not reverted; the run report
   embeds an environment snapshot; CI runs the same machinery in **create mode**. Includes
   the failure payoff: a failing suite leaves the environment in place for debugging.
6. **"Anatomy of a simple test" added** — a test is a folder: **one `config.yaml`**
   (`needs` / `uses` / `agent` / `setup` / `ci` sections) + `suite_test.go` + fixtures, and
   the runner contract. No provisioning code in the suite.
7. **"Reproducing a test's environment" added** — any test's environment is one command
   away: `e2ectl start --config <the test's config.yaml>` deploys exactly what the test runs
   on. Reproduce a CI failure locally, explore manually, or let an agent reproduce a bug.
8. **"What runs in the CI" added** — the `ci` section of each test's config declares its
   targets, agent source and triggers; **version resolution** is defined (explicit values
   > pinned classics > floating classics resolved once per run) and every run publishes a
   **resolved config** artifact, so floating "latest" references are always auditable and
   exactly reproducible; every expansion must satisfy `needs`, checked **statically
   before anything is provisioned**; pipeline jobs are generated from the configs, not the
   other way around.
9. **"Integration with the current framework" added** — grounded in the actual codebase:
   the standalone provisioning path, the Pulumi-free installers, and the stack snapshot
   already exist; Pulumi retreats behind `e2ectl` (create/destroy only); `UpdateEnv` is
   retired, not extended.
10. **"Out of scope (for now)"** — long-running QA environments, resource pooling,
   local-first test execution, migrating existing tests: parked explicitly.
11. **"Open questions"** — what was deliberately not decided.

**Decisions applied from the discussion — veto any of these:**

- Environment reset is out of v1; warm-state testing is the default (`state: warm`).
- Debugging stays in the developer toolbox (ssh/kubectl/journalctl) — no CLI debug verbs.
- `e2ectl stop` destroys by default; `e2ectl update` rebuilds changed components only.
- Test requirements are hand-written (the `needs` section), not derived from Go code.
- **One `config.yaml` per test** merging needs, canonical environment, agent, setup and CI
  definition; the classics catalog (contributable by PR, QA-maintained) serves developers,
  tests and the pipeline — templates, bases and CI targets are the same catalog.
- The repro workflow (`e2ectl start` from a test's config) is first-class and documented.
- `setup:` lives in the test config (runner applies it and cleans up its own fixtures);
  the suite is a plain Go test with context injected.

---

# My vision regarding testing and qa experience on datadog-agent

_Disclaimer: This document will not go into details about how things are implemented, it is
likely to be an extension to the existing E2E testing framework but could be something else
as well. The goal is to depict a workflow and what should be made possible._

## Why

The E2E framework is heavily used to test the agent in real environments, and it mostly
works well in the CI. The local experience is the weak spot, and the requests keep coming:

- integrations-core: a framework to easily create testing scenarios where the agent is
  deployed and we can see the metrics it emits — in the backend or in the fakeintake
  ([#52725](https://github.com/DataDog/datadog-agent/pull/52725))
- Adversarial Simulation Squad: easily spawn an instance, install a coding agent and the
  agent on the host, and interact with the environment
  ([#52247](https://github.com/DataDog/datadog-agent/pull/52247))
- agent health: define an environment once and reuse it locally, including scenarios
  extracted from E2E tests
  ([#51650](https://github.com/DataDog/datadog-agent/pull/51650))

Developers — humans and AI agents — should be able to spawn and reuse QA environments
efficiently, locally, without the framework getting in the way.

## Vision

The tooling should provide a CLI that is easily discoverable and usable by both humans and
AI agents.

The core idea is an inversion of today's model: **today an environment is ephemeral** —
provisioned per CI run, torn down afterwards, owned by the test. **In this vision an
environment is a durable asset** — owned by the developer or the agent — that you create
once, install the agent on, iterate against, and point tests at.

The environment can be anything: a remote EKS cluster, a remote VM, a local VM, a local
kind cluster, the user laptop. The installation method can be anything as well: the install
script with given params, a local .deb, a simple agent-core binary, helm, kubectl apply,
helm + operator…

## The configuration language

Everything is driven by one configuration language. An environment is described by **one
definition** with three sections — environment, agent, workloads — and each verb applies
one section of it. The definition is a single shareable artifact: drop it in a channel and
a colleague has my whole setup.

A set of **classic environment definitions** lives in a shared catalog in the repository
(`kind-latest`, `eks-1-29`, `docker-ubuntu22`…). Anyone can contribute one by PR, the QA
team maintains and reviews them. Developers start from them, tests build on them, and the
CI pipeline expands onto them — one catalog, three uses.

Common cases should need almost nothing: classics provide defaults for everything, and a
definition usually only overrides what matters.

The same language is used with two different semantics:

- **values**, when I create an environment: `kubernetes: "1.31"` — what I want;
- **constraints**, when a test declares what it needs: `kubernetes: ">=1.29"` — what is
  required.

A test's config carries both, as `needs` and `uses` (see below). Running a suite on an
environment is checking that every `needs` constraint is satisfied.

## The verbs

```
e2ectl templates                                        # browse the classic environment types
e2ectl start --config <env>.yml --name <name>           # create a named environment
e2ectl list                                             # my running environments
e2ectl install --config <env>.yml --env <name>          # apply the agent section
e2ectl deploy --config <env>.yml --env <name>          # apply the workloads section
e2ectl update --env <name>                              # rebuild changed code, redeploy in place
e2ectl fakeintake <command> --env <name>               # inspect what the agent sent
e2ectl test --suite <path> --env <name>                # run existing E2E suites on it
e2ectl stop --env <name>                                # destroy the environment
```

The config passed to `start` can be anything: one of my own definitions, a classic from the
catalog, or **a test's `config.yaml`** — deploying exactly the environment a test runs on.

The CLI stops at giving you everything needed to connect to the environment (ssh, kubectl
context…). Debugging stays in the normal developer toolbox — `kubectl logs`, journalctl,
the agent `status` command.

Now let's make it concrete with two scenarios: a developer working on a Kubernetes feature,
and a developer working on a core agent feature. Same verbs, only the configuration
changes.

## Scenario 1 — a Kubernetes feature

I'm working on a change to how the agent collects data from Kubernetes — say, the
`kubernetes_state` core check. I want to see my change work on a real cluster with my code,
without waiting for the CI.

**1. Describe my setup once, and create the environment:**

```yaml
# ksm-dev.yml — my whole QA setup in one file
environment:
  base: kind-latest       # from the classics catalog
  kubernetes:
    version: "1.31"
    nodes: 1
  fakeintake: true
agent:
  source: local        # build from my working tree
  install: helm
  config:
    kubeStateMetricsCore:
      enabled: true
    # ... any datadog.yaml / conf.d override
workloads:
  - manifest: my-app.yaml   # my own manifest or helm chart
    wire: agent             # the tooling configures it to talk to the agent
```

```
e2ectl start --config ksm-dev.yml --name ksm-dev
```

A few minutes later the cluster is up, my `kubectl` is wired to it, and the fakeintake is
running inside the cluster. `e2ectl list` shows `ksm-dev` with its age.

**2. Install my agent on it:**

```
e2ectl install --config ksm-dev.yml --env ksm-dev
```

The `agent` section is applied: the CLI builds the agent image from my working tree,
deploys the Helm chart with that image. The agent is configured with the API key from my
local config and sends data to the backend.

**3. Deploy my workload, wired to the agent:**

```
e2ectl deploy --config ksm-dev.yml --env ksm-dev
```

The `workloads` section is applied: my app runs on the cluster, configured to talk to the
agent — dogstatsd host and port, trace agent endpoint, the right environment variables.
A bare `kubectl apply` is unlikely to be correctly configured; that should not be my
problem.

**4. The iteration loop — where most of my time goes:**

```
e2ectl fakeintake metrics --env ksm-dev --name 'kube*' --json
e2ectl fakeintake tail --env ksm-dev
# ... I change code ...
e2ectl update --env ksm-dev
```

`update` detects the components that changed — here, the agent — rebuilds only those, and
redeploys them in the same cluster. A couple ofOk n minutes per cycle. The fakeintake keeps its
data across updates, so before/after is diffable (until I run a test suite, which starts
from a clean fakeintake). When a metric doesn't show up, I debug with `kubectl logs` and
the agent `status` command, through the access the CLI already gave me.

If I need this against a real cloud cluster instead, I change one line — `base: eks` —
and run the same scenario on EKS.

**5. Run the existing test suites against my live cluster:**

```
e2ectl test --suite ./test/new-e2e/tests/containers/... --env ksm-dev
e2ectl test --suite ./test/new-e2e/tests/containers/... --run TestOneSpecificBehavior --env ksm-dev
```

No provisioning: the suite's `needs` are checked against my environment, and the tests run
as-is on it.

**6. End of day:**

```
e2ectl stop --env ksm-dev
```

The environment is destroyed. If I want to keep it warm for a few days, I simply don't stop
it — it's my environment.

## Scenario 2 — a core agent feature

I'm working on a change inside the agent core. I need a regular Linux host running my
build, not a cluster.

**1. Describe my setup once, and create the environment:**

```yaml
# core-dev.yml
environment:
  base: ec2-ubuntu-latest   # from the classics catalog
  vm:
    os: ubuntu-22.04
    arch: amd64
  fakeintake: true
agent:
  source: local
  install: binary          # no full .deb needed for core agent work
  config:
    # ... datadog.yaml overrides
workloads:
  - manifest: my-app.yaml
    wire: agent
```

```
e2ectl start --config core-dev.yml --name core-dev
```

VM up in a few minutes, ssh details printed, and the fakeintake is reachable from my
laptop.

**2. Install my agent on it:**

```
e2ectl install --config core-dev.yml --env core-dev
```

The CLI builds the agent binary from my working tree, copies it to the VM, configures it
and runs it. No package build involved — for core agent work I only care about the binary
anyway. API key from my local config, data to the backend.

**3. Deploy my workload, wired to the agent:**

```
e2ectl deploy --config core-dev.yml --env core-dev
```

Same `workloads` section as the Kubernetes scenario: my app runs on the VM, configured to
talk to the agent — the tooling injects the agent endpoints, again not my problem. If I
prefer to do things by hand, the ssh access is mine.

**4. The iteration loop:**

```
e2ectl fakeintake metrics --env core-dev --json
# ... I change code ...
e2ectl update --env core-dev
```

The changed binaries are rebuilt, copied to the VM, the agent restarts. Under a minute
per cycle. Debugging through ssh, journalctl, the agent `status` command.

**5. Run the existing test suites:**

```
e2ectl test --suite ./test/new-e2e/tests/host/... --env core-dev
```

**6. End of day:**

```
e2ectl stop --env core-dev
```

## What both scenarios share

Both developers use the same verbs, in the same order, against environments of completely
different natures — a local kind cluster and a cloud VM. The only difference is the
content of one small configuration file each. That is the point: the CLI is a thin,
discoverable layer on top of a single configuration language, and environments are just
configurations. That file is also the sharing unit: drop it in a channel and a colleague
has my whole setup.

The full loop for both: start the environment once, install the agent, iterate on your
code with `e2ectl update` and inspect the fakeintake, and run the relevant test suites
against the same environment whenever you want.

## Running tests on an existing environment

Each test declares what it needs in the `needs` section of its config — constraints in the
same configuration language as environment definitions:

```yaml
# config.yaml (excerpt) — next to the suite
environment:
  needs:                          # the contract — checked before the run
    kubernetes: ">=1.29"
    agent:
      installed: true
      state: warm                 # or fresh: needs a freshly installed agent
    fakeintake: true
  uses:                           # the canonical environment — deployed in create mode
    base: kind-latest
    kubernetes: {version: "1.31"}
```

```
e2ectl test --suite ./test/new-e2e/tests/containers/... --env ksm-dev
```

The CLI checks the live environment against the `needs` — probing the actual state
(cluster version, agent health, fakeintake reachability), not just trusting configuration
files — and fails early with a clear error when something is missing (e.g. "this suite needs
a Kubernetes environment").

The test definitions themselves stay unchanged; a suite just needs its config to be
runnable this way.

**How a run interacts with a warm environment:**

- `needs` can also express **freshness**: `agent: {state: warm}` (default) or
  `agent: {state: fresh}`. A suite that needs a freshly installed agent fails fast on a
  warm environment ("installed 2 days ago, this suite needs a fresh agent"). Suites that
  test the installation flow itself just declare `state: fresh` — they naturally stay
  create-mode tests, no special casing.
- The fakeintake is wiped at the start of each run, so assertions run against a clean
  fakeintake even on a warm environment. The previous content can be archived to a file
  before the wipe, so iteration history is not silently lost.
- The environment is locked for the duration of the run: one test run at a time per
  environment, the others fail fast ("env busy"). Parallelism comes from named
  environments, not from concurrent runs on one environment.
- Tests may deploy workloads and reconfigure the agent; changes are **not** reverted after
  the run. Drift is the accepted price of reuse — surfacing and resetting drift are the
  antidote (see open questions).
- The run report embeds an environment snapshot: what was installed (components, source,
  git sha), the environment definition, and what drifted. On a failing run, that snapshot
  is the reproducibility artifact.

And the payoff of the whole design: when a test fails on my environment, **the environment
stays**. I can inspect the fakeintake, check the agent, rerun a single test in seconds —
the failure scene persists instead of being torn down before I even open the logs.

In the CI, the same machinery runs in create mode: the test's `uses` environment is
deployed (or its `ci` targets expanded onto, see below), the suite runs, the environment is
destroyed. The CI and the local loop are the same tool in two modes.

## Anatomy of a simple test

What a future suite looks like — **a test is a folder**: one config, pure-assertion Go
tests, fixtures. No provisioning code.

```
test/new-e2e/tests/kube-state/
  config.yaml
  suite_test.go
  fixtures/
    kube-state-metrics.yaml
```

```yaml
# config.yaml — the test's whole world
environment:
  needs:                        # what the suite requires from any environment
    kubernetes: ">=1.29"
    agent:
      installed: true
      state: fresh             # needs a freshly installed agent
    fakeintake: true
  uses:                         # its canonical environment, deployable as-is
    base: kind-latest
agent:
  source: local                 # overridden to `pipeline` by the CI
  install: helm
  config:
    kubeStateMetricsCore:
      enabled: true
setup:                          # applied by the runner before the run
  workloads:                    # same workload language as environment definitions
    - manifest: fixtures/kube-state-metrics.yaml
      wire: agent
ci:                             # where this test runs in the CI — see below
  targets: [kind-latest, eks-1-29]
  agent: {source: pipeline}
  triggers: [merge_request, nightly]
```

```go
// suite_test.go — pure assertions, no provisioning
func TestKubeStateSuite(t *testing.T) {
    e2e.Run(t, &suite{})
}

func (s *suite) TestMetricsAreEmitted() {
    require.Eventually(s.T(), func() bool {
        metrics, _ := s.Fakeintake().GetMetric("kubernetes_state.node.count")
        return len(metrics) > 0
    }, 2*time.Minute, 5*time.Second)
}
```

Notice what is gone compared to today: no provisioner args, no agent params struct, no
fakeintake deployment code in the suite. The suite is "given a live environment with these
properties, assert X". Everything environmental lives in the config.

**The runner contract** — what `e2ectl test` does, in order:

1. **Verify** `needs` against the live environment (reuse mode) or deploy the `uses`
   environment (create mode); fail fast either way
2. **Apply** `setup` — the agent config delta and the fixture workloads, with the same
   machinery as `install` and `deploy`
3. **Wipe** the fakeintake
4. **Run** the suite — the tests get the environment context injected (fakeintake URL,
   kubeconfig, ssh)
5. **Report** — results plus the environment snapshot

What the runner deployed from `setup`, the runner removes; what the test code mutates at
runtime, stays (the test should clean up after itself). Only the runner knows exactly what
it did.

**Two execution paths, same folder:**

- Local: `e2ectl test --suite ./tests/kube-state/... --env ksm-dev` — verify `needs`
  against my warm environment (a `state: fresh` suite fails fast here, honest and clear).
- CI: `e2ectl test --suite ./tests/kube-state/...` — deploy the `uses` environment, run,
  destroy; or expand onto the `ci` targets for the matrix.

Suites that never migrate keep their Go provisioning and run in create mode only. New
tests are born reusable; old ones become reusable when someone adds the config.

## Reproducing a test's environment

A test's config **is** an environment definition — so any test's environment can be
deployed directly, without running the test:

```
# a CI test failed. Reproduce it exactly:
e2ectl start --config test/new-e2e/tests/kube-state/config.yaml --name ksm-repro
e2ectl install --config test/new-e2e/tests/kube-state/config.yaml --env ksm-repro
e2ectl test --suite ./tests/kube-state/... --env ksm-repro --run TestThatFailed
# ... debug in place, the environment is mine ...
```

Any test's environment is one command away — for triaging a CI failure, for exploring a
scenario by hand, or for an AI agent trying to reproduce a bug. No extraction step, no
translation: the environment the test runs on is the environment I just started. This is
exactly the reuse of E2E scenarios that the agent-health team asked for
([#51650](https://github.com/DataDog/datadog-agent/pull/51650)).

For the exact reproduction of a CI failure, start from the **resolved config** the failed
job published (see "What runs in the CI") — the versions are then identical, not
re-resolved.

## What runs in the CI

`needs` alone cannot determine what the CI should create — it is a predicate, not a choice.
The choice lives in the test's config, in the `ci` section, and in the classics catalog.

**Classics.** The shared catalog of concrete environment definitions — `kind-latest`,
`eks-1-29`, `docker-ubuntu22`… — is what the CI expands onto. It is the same catalog
developers start from and tests build on: there is no such thing as "a CI environment"
that behaves differently from mine.

**The `ci` section** declares where the test runs, with which agent source, under which
triggers:

```yaml
ci:
  targets: [kind-latest, eks-1-29]   # each must satisfy `needs`
  agent: {source: pipeline}          # the artifact built by this pipeline run
  triggers: [merge_request, nightly]
```

The `agent.source` is pipeline context: `pipeline` (the artifact built by this run),
`{channel: "7.64.x"}`, `{version: "7.63.0"}` — released versions for upgrade paths, for
example.

**Version resolution.** A config does not need to pin versions, but something must be
concrete at provision time. Resolution order:

1. an explicit value in `uses` wins — fully deterministic;
2. otherwise, the classic it builds on — classics either pin (`kind-1-29`) or float
   (`kind-latest`, mapped by the catalog maintainers to a concrete version);
3. a floating classic is resolved **once per pipeline run**, so every test in the run
   provisions the same version — and each resolution is validated against `needs`.

`needs` never provisions anything by itself; it only validates. A config whose `uses` and
`ci` targets are all floating still gets a concrete answer — it is decided at run time
instead of author time.

And the auditability guarantee: every run publishes a **resolved config** — the fully
concrete config actually used (environment versions, agent version and source) — as an
artifact of the job. The report always says exactly what ran, even when the config said
"latest"; and reproducing a CI failure starts from the resolved config, so versions are
identical, not re-resolved.

Practical policy by default: `merge_request` runs prefer pinned classics — a PR failure
should mean my change, not a catalog move; `nightly` runs take the floaters — that is where
new versions get caught before releases.

**Static validation before provisioning.** Every pairing — `uses`, and each `ci` target —
must satisfy the test's `needs`, and this is checked **before anything is provisioned**:
a suite that needs Kubernetes paired with a VM target fails the pipeline in seconds, not
ten minutes after provisioning. `e2ectl plan validate` checks every test's config across
the repository; `e2ectl plan list` aggregates all configs into the answer to "what runs in
the CI tonight?" — generated, not maintained.

The actual pipeline jobs are generated from these configs. The per-test `config.yaml`
files are the source of truth; the GitLab plumbing becomes a thin generator, not the other
way around.

## Integration with the current framework

This is an extension of the existing E2E framework, and the seams it needs already exist:

- The client and component layers no longer depend on `testing.T`, so provisioning can be
  driven from a standalone binary (`testing/standalone`, used today by `cmd/ai-sandbox`).
  `e2ectl` is another consumer of that same path.
- Agent installation is already decoupled from Pulumi (`testing/installers`: helm, install
  script): it works on initialized environments and takes agent configuration as
  parameters. This is the engine behind `e2ectl install`, the `setup` section, and agent
  config changes at run time.
- A stack snapshot — a single JSON file describing the environment's resources — can
  already rehydrate a typed environment without any Pulumi interaction. The snapshot
  becomes the currency of the named-environment registry, the CI's resolved-config
  artifact, and the repro workflow's entry point: one artifact, three uses.
- `e2e.WithDevMode` (keep infrastructure alive after a test) shows the demand for durable
  environments.

**The architectural rule: Pulumi lives only at the creation and destruction of
environments, behind `e2ectl`.** Test execution never touches a provisioner. In reuse mode
the runner wires the environment from the snapshot and injects context (fakeintake URL,
kubeconfig, ssh) into the test process; tests shrink to libraries — fakeintake client,
remote host and Kubernetes helpers — with no provisioning code and no provisioner
plumbing. In create mode, provisioning runs first and produces the same snapshot; the
execution path is then identical. One execution path, two modes.

`s.UpdateEnv` — today's Pulumi-coupled way to change the agent mid-suite — is retired by
this design, not extended: its legitimate use cases (toggle a check, change an interval
mid-test) are served by the same Pulumi-free installers. During the migration, a legacy
suite can attach to a named environment through the snapshot-based adapter with a single
provisioner line; the target model has no provisioner concept at all on the test path.

Sequencing:

1. `e2ectl` over the standalone provisioning path: `start`/`list`/`stop`/`install` plus
   fakeintake inspection; config language for the environment section; named-environment
   registry. No test-side changes.
2. Reuse mode: snapshots as the registry's currency; test runs receive wired context;
   `needs` probing; `UpdateEnv` retires.
3. CI mode: job generation from `config.yaml`, snapshots published as resolved-config
   artifacts, `plan validate`/`plan list`, fakeintake wipe, environment locks.
4. Tightening: component-scoped `update`, classics catalog consolidation, repro
   documentation.

Custom environments (`tests/npm`, `tests/ha-agent`, …) remain Go-provisioned and run in
create mode only; the config language v1 covers stock environments. Legacy suites keep
their authoring style; new tests are born config-driven — the two coexist until suites
migrate.

## Out of scope (for now)

This document focuses on local iteration and testing. Related but separate goals:

- Long-running QA environments
- Resource pooling (limited-capacity platforms such as MacOS, AIX)
- Running tests locally instead of CI / local-first test execution
- Migrating existing E2E tests

The CLI and its configuration language are designed so these can layer on later.

## Open questions

- Environment reset: ability to reset a given environment to a clean state — the natural
  satisfier of `agent: {state: fresh}`
- Drift detection: environments are mutated by tests and manual changes; how do we surface
  what is deployed/installed?
- Cost surfacing for remote environments (what's running, for how long)
- Fakeintake inspection primitives: get by name, tail, filter, `--json` first (TUI later)
- What "usable by AI agents" means concretely: `--json` everywhere, machine-readable config
  schema, MCP server, repository skill?
- What "the user laptop" as an environment actually supports (systemd agent? containers?
  local fakeintake?)
- CI failure evidence: should a failing CI run keep its environment alive (with a TTL) for
  triage?
- Migration pace: when do legacy suites move from `e2e.Run` + provisioners to context
  injection, and what is the deprecation timeline for `UpdateEnv`?
- Config evolution: how do classics version? What happens to tests pinned to an old
  classic when it is retired?
