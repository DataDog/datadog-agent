# Reusable QA environments: what we built and where it's going

A concise summary of the work on changing how Agent tests are provisioned and executed —
for developers who want to use it or extend it. Deep dives: [plan index](qa-e2ectl-plans-index.md),
[CLI README](../test/e2e-framework/cmd/e2ectl/README.md).

## The problem

E2E environments today are ephemeral and owned by the test run: provision → assert →
destroy, once, in CI. That makes local iteration the weak spot: reproducing a CI failure
means re-running the whole pipeline, a broken Agent can't be inspected where it failed,
and "define an environment once, reuse it" — a repeatedly requested workflow — isn't
possible without hand-rolled scripts.

The change: environments become **durable, named assets** a developer (or agent) creates
once, installs the Agent on, iterates against, and points tests at — with one CLI and one
YAML config language, locally and in CI.

## e2ectl today

```sh
e2ectl environments                    # what environment types exist
e2ectl init --base kind --output my.yaml   # annotated starter config, generated from typed Go structs
e2ectl start   --config my.yaml --name dev    # provision (kind locally; EC2 via the executor)
e2ectl install --env dev                       # install the Agent (Helm / official script)
e2ectl update  --env dev                       # rebuild your change and redeploy (kind dev loop)
e2ectl fakeintake metrics --env dev            # what the Agent actually sent
e2ectl list / stop --env dev
```

Two flows work today:
- **Local kind** — the full loop is Pulumi-free and fast: cluster + local fakeintake,
  `update` rebuilds a dev image and upgrades the chart, metrics visible in the fakeintake.
  Verified live (rename-a-metric → `update` → renamed metric observed).
- **EC2 host** — VM plus ECS Fargate fakeintake via a Pulumi executor child process, then
  the official install script in-process. Implemented and tested offline; the live AWS
  smoke is gated on credentials.

Requirements are the usual ones: Docker + kind for local, the runner profile
(`~/.test_infra_config.yaml` or `E2E_API_KEY`) for the API key — credentials are never
written into configs. State lives in `$E2ECTL_HOME` (default `~/.e2ectl`), one directory
per environment. Build with
`bazel build //test/e2e-framework/cmd/e2ectl:e2ectl` (+ `:e2ectl-worker` for EC2).

## The design rules that make it work

1. **The CLI never links Pulumi.** Cloud provisioning runs in a separate executor
   binary (`e2ectl-worker`); every other operation — kind, install, update, inspection —
   is in-process and starts in milliseconds. A stale executor is refused by a protocol
   version check.
2. **The snapshot is the currency.** One private JSON per environment (resources +
   component bindings). Every command after `start` reattaches from it — no
   re-provisioning, no Pulumi — and the same artifact is what a future test-run
   attachment will consume.
3. **Configuration is typed and self-describing.** `environment.base` selects a driver
   whose typed Go struct owns its section (`kind:`, `ec2-host:`); `agent.install` selects
   an installer whose typed struct owns its section (`script:`, `helm:`). One schema engine
   derives validation, defaults and the annotated starter config — unknown fields fail
   with actionable messages, and fields an installer doesn't consume (e.g. `image` for a
   script install) simply don't exist in its section.
4. **Agent installation is not provisioning.** Installers (Helm chart, install script)
   run against an attached environment, outside Pulumi, the same way for every backend —
   so iterating on Agent code never re-provisions infrastructure.
5. **Fakeintake follows the provisioning backend** — Pulumi (ECS Fargate) for cloud,
   local Docker for kind — and inspection works on any environment while it lives.
6. **Adding an environment is registration, not plumbing.** A data-only config struct +
   schema, a driver implementation, one `Define(...)` line. No switches in commands, no
   hand-written templates, no per-cloud fields threaded through generic code.

## What's in flight

| Direction | One-liner | Status |
|---|---|---|
| EKS environment | Reuse the framework's existing EKS Pulumi scenario (Linux/Windows node groups) behind the same CLI | Designed, not implemented |
| Local host agent | The laptop as the environment: `go build`-built Agent run in a local container (isolated, portable), wired to a local fakeintake — the fastest possible loop | Designed, not implemented |
| Receiver selection | Choose fakeintake vs. the real backend explicitly; today routing is inferred from fakeintake presence | Designed, not implemented |
| Custom/multi-Agent environments | Scenarios expose their own typed config (two Agents, two fakeintakes…) and their own installer; the CLI stays single-agent-generic | Designed, not implemented |
| Agent-config typing remainder | Contract revision so installers receive typed sections end-to-end; scenario agent sections | Partially implemented |
| Test execution | Run existing suites against a live environment; needs-probing; CI job generation from configs | Vision, not started |

Everything above has a concrete plan with status and boundaries in the
[plan status index](qa-e2ectl-plans-index.md); the implemented ledger distinguishes what
exists, what's verified, and what remains.
