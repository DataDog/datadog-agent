# My vision regarding testing and QA experience on datadog-agent

> Local working draft — the live version lives on
> [Confluence](https://datadoghq.atlassian.net/wiki/spaces/~7120201870126a495245b69e47156354de0ad9/pages/7146275864/My+vision+regarding+testing+and+qa+experience+on+datadog-agent).
> Status: local draft v9 (test-run interaction contract).
> **Superseded** by `qa-vision-confluence-update.md` — the update proposal for the
> Confluence page, which now carries the latest content (CI plan, test anatomy).

_Disclaimer: This document will not go into details about how things are implemented, it is
likely to be an extension to the existing E2E testing framework but could be something else
as well. The goal is to depict a workflow and what should be made possible._

## Why

The E2E framework is heavily used to test the agent in real environments, and it mostly works
well in the CI. The local experience is the weak spot, and the requests keep coming:

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

Common cases should need almost nothing: templates provide defaults for everything, and a
definition usually only overrides what matters.

The same language is used with two different semantics:

- **values**, when I create an environment: `kubernetes: "1.31"` — what I want;
- **constraints**, when a test suite declares what it needs: `kubernetes: ">=1.29"` — what
  is required.

Running a suite on an environment is checking that every constraint is satisfied.

## The verbs

```
e2ectl templates                                        # browse available environment types
e2ectl start --config <env>.yml --name <name>           # create a named environment
e2ectl list                                             # my running environments
e2ectl install --config <env>.yml --env <name>          # apply the agent section
e2ectl deploy --config <env>.yml --env <name>          # apply the workloads section
e2ectl update --env <name>                              # rebuild changed code, redeploy in place
e2ectl fakeintake <command> --env <name>               # inspect what the agent sent
e2ectl test --suite <path> --env <name>                # run existing E2E suites on it
e2ectl stop --env <name>                                # destroy the environment
```

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
  template: kind
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
redeploys them in the same cluster. A couple of minutes per cycle. The fakeintake keeps its
data across updates, so before/after is diffable (until I run a test suite, which starts
from a clean fakeintake). When a metric doesn't show up, I debug
with `kubectl logs` and the agent `status` command, through the access the CLI already gave
me.

If I need this against a real cloud cluster instead, I change one line — `template: eks` —
and run the same scenario on EKS.

**5. Run the existing test suites against my live cluster:**

```
e2ectl test --suite ./test/new-e2e/tests/containers/... --env ksm-dev
e2ectl test --suite ./test/new-e2e/tests/containers/... --run TestOneSpecificBehavior --env ksm-dev
```

No provisioning: the suite's declared requirements are checked against my environment, and
the tests run as-is on it.

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
  template: ec2
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

Each test suite declares what it needs in a yaml file, written by hand next to the suite,
using the same configuration language as the environment definition — but expressing
constraints rather than values:

```yaml
# requirements.yml — next to the suite
requires:
  kubernetes: ">=1.29"
  agent:
    installed: true
    state: warm            # or fresh: needs a freshly installed agent
  fakeintake: true
  os: linux
```

```
e2ectl test --suite ./test/new-e2e/tests/containers/... --env ksm-dev
```

The CLI checks the live environment against these requirements — probing the actual state
(cluster version, agent health, fakeintake reachability), not just trusting configuration
files — and fails early with a clear error when something is missing (e.g. "this suite needs
a Kubernetes environment").

The test definitions themselves stay unchanged; a suite just needs its requirements file
to be runnable this way.

**How a run interacts with a warm environment:**

- Requirements can also express **freshness**: `agent: {state: warm}` (default) or
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

In the CI, the same machinery runs in create mode: with no `--env`, the CLI concretizes the
suite's requirements into an environment, creates it, runs the suite and destroys it. The
CI and the local loop are the same tool in two modes.

## Out of scope (for now)

This document focuses on local iteration and testing. Related but separate goals:

- Long-running QA environments
- Resource pooling (limited-capacity platforms such as MacOS, AIX)
- Running tests locally instead of in CI / local-first test execution
- Migrating existing E2E tests

The CLI and its configuration language are designed so these can layer on later.

## Open questions

- Environment reset: ability to reset a given environment to a clean state
- Drift detection: environments are mutated by tests and manual changes; how do we surface
  what is deployed/installed?
- Cost surfacing for remote environments (what's running, for how long)
- CI failure evidence: should a failing CI run keep its environment alive (with a TTL) for
  triage?
- Fakeintake inspection primitives: get by name, tail, filter, `--json` first (TUI later)
- What "usable by AI agents" means concretely: `--json` everywhere, machine-readable config
  schema, MCP server, repository skill?
- What "the user laptop" as an environment actually supports (systemd agent? containers?
  local fakeintake?)
