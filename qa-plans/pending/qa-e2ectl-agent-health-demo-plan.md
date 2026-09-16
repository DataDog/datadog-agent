# Supporting the agent-health demo/QA environments use case (PR #56182) with e2ectl

> **Category C — pending plan.** What e2ectl would need to support the use case
> PR #56182 serves: standalone demo/QA environment provisioning with scenario
> actions. Analysis only — no code changes. See the
> [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** plan only, grounded in the PR diff (v2174 lines reviewed).
**PR:** [DataDog/datadog-agent#56182](https://github.com/DataDog/datadog-agent/pull/56182)
— *feat(agent-health): standalone Cobra CLI for agent-health demo/QA environments*.

## 1. What the PR does

A self-contained `agent-health-demo` Cobra CLI (`test/new-e2e/tests/agent-health/cmd/`)
that provisions **realistic agent-health demo hosts** for QA and demo purposes:

| Command | What it does |
|---|---|
| `create` | Provisions an EC2 VM (Ubuntu) via the e2e framework **in-process** (Pulumi Automation API); credentials, API key and passphrase from the runner profile (`~/.test_infra_config.yaml`) |
| `run` | Executes a scenario **action** over SSH on the most recent host: `--action retrigger` triggers the issue, `--action remediate` fixes it |
| `list` / `delete` | Local JSON record store of provisioned environments |

Key pieces:

- **Scenarios as data**: `demo.go` defines `DemoScenarios` — named scenarios
  (`docker-permissions`, `invalid-config`) each with an issue, a description,
  and SSH `Actions` (shell command lists). E.g. retrigger `docker-permissions`
  = `sudo gpasswd -d dd-agent docker && sudo systemctl restart datadog-agent`.
- **Pulumi scenario**: `scenario.go` registers `aws/agent-health-demo` with the
  (newly extensible) framework registry; `AgentHealthDemoRun` dispatches on the
  `demolab:demoScenario` Pulumi config key to `ec2.VMRunWithParams(...)` with
  per-scenario agent config (embedded YAML fixtures; e.g. health-platform
  forwarder interval shortened to 30s, private-action-runner with the rshell
  remediation action allowlisted).
- **Registry extension** (`registry/scenarios.go`): `RegisterScenario` +
  `builtinScenarioFuncs` split + a `go generate` tool (`tools/generate-scenario-imports`)
  producing `scenarios_import_gen.go` so the Pulumi run binary blank-imports
  test-package scenario registrations.
- **Multi-host**: `--count N --suffix index|random` provisions N stacks.
- **SSH plumbing**: key loading into ssh-agent, AWS SSO helper.

## 2. The overlap with e2ectl — this is a second QA-environments CLI

| Concern | PR #56182 `agent-health-demo` | e2ectl (current) |
|---|---|---|
| Provision infra | In-process Pulumi Automation API (the CLI links the whole framework + Pulumi SDK — *"it is large"*) | Pulumi isolated in the `e2ectl-worker` child process; the CLI is Pulumi-free |
| Environment record | Local JSON store (id, scenario, host IP) | `$E2ECTL_HOME/envs/<name>` + snapshot (resources, bindings, fakeintake, agent state) |
| Environment lifecycle | `create` / `list` / `delete` | `start` / `list` / `stop` (+ `install`, `update`, `test`, `fakeintake`) |
| Scenario definition | Go data (`DemoScenarios` map) + Pulumi config keys | Typed YAML config (`environment.base` + installer sections + `workloads:`) |
| Drive-the-host | `run --action retrigger` (SSH command lists) | *(missing — no action/step concept)* |
| Agent installation | Bundled into provisioning (Pulumi params) | Explicit `install`/`update` steps, outside Pulumi |
| Multi-host | `--count` / `--suffix` | Named environments (shell loop today) |
| Consumers | agent-health team demos/QA; shows issues + remediation flows | QA iteration loops + existing test suites attach |

The PR's motivation — *"give agent teams a way to spin up realistic demo/QA hosts
that stays close to the e2e framework, reusing the same provisioners, profile
and config the automated e2e tests use"* — is e2ectl's mission statement.

## 3. What the use case needs that e2ectl doesn't have yet

1. **Scenario actions** — named, ordered command bundles run against a live
   environment's host (`retrigger`, `remediate`). e2ectl has no post-install
   "drive the environment" command.
2. **Scenario presets** — the agent-health scenarios bundle: an EC2 VM with a
   specific agent config (health platform, private action runner, shortened
   forwarder interval), fixture files, and their action sets. e2ectl expresses
   agent config per installer, but has no first-class "scenario" bundling
   environment + agent + workloads + actions under one name.
3. **Registration of team-owned scenarios** — the PR's `RegisterScenario` +
   generated imports makes test packages able to *own* scenarios in Go. e2ectl's
   equivalent (driver registry + typed config) lives in the CLI's source tree;
   teams cannot contribute a scenario from their test package today.
4. **`--count` multi-host** — cheap convenience over named environments.
5. **Hosts reporting to the real backend** (site, api-key, tags passthrough) —
   e2ectl's EC2 host flow exists; the demo adds `--site`/`--tags` selection.

## 4. Options

### Option A — adopt the PR as-is; converge later
Let a second CLI live. Cost: two stores, two provisioning paths, two UXs for
the same QA population; the demo CLI links Pulumi (heavy binary, every user
needs the pulumi CLI at runtime); drift.

### Option B — support the use case in e2ectl (recommended)
The PR's *scenario content* is exactly the kind of thing e2ectl should run;
the PR's *plumbing* (in-process Pulumi, JSON store, SSH runner) duplicates
what e2ectl already separates better. Converge on e2ectl:

1. **Scenario presets**: `e2ectl start --scenario docker-permissions --name demo1`.
   A preset is a packaged config: base `ec2-host`, an agent section (script
   installer with the scenario's `datadog.yaml` overlay + private-action-runner
   flags), workloads (docker + busybox compose), and its action set. Presets
   can be contributed as data (YAML in the repo) — keeping e2ectl's
   registration philosophy — or, closer to the PR, as Go definitions in the
   owning team's package with a small `registry.RegisterScenarioPreset` in the
   e2ectl-worker (which already owns Pulumi and could blank-import them the
   same way the PR's run binary does).
2. **`e2ectl run --env demo1 --action retrigger`**: executes a scenario
   action's command list through the environment's attached `RemoteHost`
   (the same transport used by tests — SSH for EC2, docker exec for local).
   Actions come from the preset; the command surface stays small and
   inspectable (`e2ectl run --list-actions`).
3. **`--count N`**: sugar looping `start` with `--name <base>-<i>`; each
   environment is independent in the store (already true by construction).
4. **Backend selection**: today routing is inferred (fakeintake present →
   fakeintake); demo hosts need the real backend. This is precisely the
   receiver-selection plan (pending) — a `receiver:` section naming the
   destination per environment. The demo preset selects the real backend
   with `--site`/api-key/tags passthrough.
5. **Private action runner**: the rshell remediation allowlist is agent
   config; with the script installer it is another config overlay — no new
   mechanism needed.

What the PR keeps contributing regardless: the **scenario registry extension**
(`RegisterScenario` + generated imports) is good framework infrastructure —
e2ectl-worker can consume it to dispatch scenario presets; the agent-health
scenario definitions (`demo.go`, fixtures) port into a preset nearly
verbatim.

### Option C — hybrid
Merge the PR, but have `agent-health-demo` become a thin skin over e2ectl
(like `e2ectl test` is over `go test`): keep the Cobra UX the team wants,
drop the in-process Pulumi/SSH/store duplication by delegating to e2ectl.
Cheapest for the team's workflow; still one environment system.

## 5. Recommended shape (B, with C as the transition)

```
e2ectl scenarios                     # list presets (agent-health demo among them)
e2ectl start --scenario docker-permissions --name demo1 --count 3
e2ectl run  --env demo1 --action retrigger     # transport-transparent
e2ectl run  --env demo1 --action remediate
e2ectl stop --env demo1
```

Concretely:

| e2ectl piece | Change | Size |
|---|---|---|
| `scenario` preset concept | Config template + actions, registered by name (data first; Go registration via the worker later) | medium |
| `e2ectl run` | New command; dispatches action commands via the attached RemoteHost | small — the executor already exists |
| `--count` | Loop over `start` | small |
| receiver selection | The pending receiver-wiring plan (needed for real-backend demos) | medium (already planned) |
| registry consumption | e2ectl-worker registers team scenarios via the PR's `RegisterScenario` machinery | small once the PR lands |

Sequence: land the PR (it unblocks the agent-health team now), then build
`e2ectl run` + scenario presets, then offer the thin-skin migration (C).

## 6. Risks / open questions

- **Two CLIs in the interim** — acceptable if the convergence path is stated
  up front; the alternative (blocking the PR) costs the team their workflow.
- **Pulumi config keys as scenario parameters** (`demolab:demoScenario`) vs
  e2ectl's typed sections — presets map them; no new union type needed.
- **Security posture**: actions run privileged shell commands by design; the
  preset registry should keep actions visible (`e2ectl run --dry-run`).
- **Multi-host scaling**: N EC2 VMs for demos is cost-sensitive; `--count`
  should stay explicit (no silent fan-out).
