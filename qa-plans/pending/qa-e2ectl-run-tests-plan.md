# e2ectl: run tests against live environments

> **Category C — pending feature; not implemented.** The vision's test-runner:
> `e2ectl test --suite <path> --env <name>` runs existing E2E suites against
> a live, e2ectl-owned environment. See the
> [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** design only; no application code changes.
**Built on:** the transport-transparent `RemoteHost` (SSH or docker exec) and the
`StaticStackProvisioner` attachment we just verified live.

## 1. What already works (the proof)

```bash
# Environment running, agent installed:
E2ECTL_LOCAL_ENV=val go test -v -tags test ./tests/agent-metric-emission/ -run TestMetricEmissionOnLocal
# → 4/4 PASS, 0.07s
```

The test attaches via `StaticStackProvisioner`, reads the snapshot, and the
transport-transparent `RemoteHost` handles the rest. The missing piece is
UX: the developer has to know the env var, the build tag, the `-run` pattern,
and the test target. `e2ectl test` wraps that into one command.

## 2. The command

```
e2ectl test --suite ./tests/agent-metric-emission/... --env val
e2ectl test --suite ./tests/agent-metric-emission/... --env val --run TestAgentHeartbeat
e2ectl test --suite ./tests/containers/... --env ksm-dev
```

What it does (thin wrapper over the proven mechanism):

1. Resolve `--env val` to the snapshot path (`$E2ECTL_HOME/envs/val/snapshot.json`)
2. Verify the environment is ready and the agent is installed
3. Set `E2ECTL_ENV=val` (and `E2ECTL_HOME`) so the test attaches
4. Invoke `go test -tags test` (or `dda inv new-e2e-tests.run`) with the
   right `-run` pattern for the environment's base
5. Stream the output; report the result; **never destroy the environment**

The key insight: e2ectl is NOT a new test runner — it's a **convenience
dispatcher** over the existing `go test` / `dda inv` infrastructure. Tests
don't change; the CLI just removes the boilerplate.

## 3. How tests declare e2ectl-attachability

The test we just wrote uses a pattern that should become the standard:

```go
func TestMetricEmissionOnLocal(t *testing.T) {
    envName := e2ectlenv.RequireEnv(t) // skips if E2ECTL_ENV not set
    snapshot := e2ectlenv.SnapshotPath(envName)
    e2e.Run(t, &metricSuite{},
        e2e.WithProvisioner(provisioners.NewStaticStackProvisioner[...]("attach", snapshot)))
}
```

A small shared package (`test/new-e2e/utils/e2ectlenv/`) provides:

```go
// RequireEnv returns the e2ectl environment name, or skips the test.
// Tests that can run against a live e2ectl environment call this in their
// entry point; the test body stays unchanged between CI and local.
func RequireEnv(t *testing.T) string

// SnapshotPath returns the snapshot path for a named environment.
func SnapshotPath(envName string) string

// Attach returns a provisioner for the named environment — one line.
func Attach[Env any](envName string) provisioner.TypedProvisioner[Env]
```

Tests that don't call `RequireEnv` are CI-only — no change needed to existing
tests. Tests that do call it gain the local-iteration capability.

## 4. The base-type problem (and its honest answer)

Today the test has two entry points (`TestMetricEmissionOnHost` for EC2,
`TestMetricEmissionOnLocal` for local). The vision's future is needs-based
selection: one test, the runner picks the environment. But that requires
the "needs" configuration language — a separate feature.

For now, `e2ectl test` selects the entry point by base type:

| `--env val` base | `--run` pattern e2ectl uses | Why |
|---|---|---|
| `local` | `OnLocal` suffix | the test uses `Attach` via `RequireEnv` |
| `ec2-host` | `OnHost` suffix | the test may need real-host capabilities |
| `kind` | not supported yet | needs the `environments.Kubernetes` type, same pattern |

This is honest: it doesn't pretend tests are base-agnostic when they're not.
The needs-based selection (one test, any environment) is the vision's M3+.

## 5. What `e2ectl test` must NOT do

- **Provision**: the environment must already exist (e2ectl start + install).
  If not, error with the commands to run.
- **Destroy**: the environment is long-lived, owned by e2ectl.
- **Rebuild**: if the agent needs a code change, that's `e2ectl update`, a
  separate command. `test` runs against what's running.
- **Hide the test framework**: the output is `go test` output, not a wrapper
  summary. Developers see what they'd see with raw `go test`.

## 6. Implementation

| Step | What | Gate |
|---|---|---|
| 1. `e2ectlenv` helper package | `RequireEnv`, `SnapshotPath`, `Attach[Env]` | The metric-emission test uses it instead of raw env-var reading |
| 2. `e2ectl test` command | Resolve env, verify ready, set env vars, shell out to `go test` | `e2ectl test --env val --suite ./tests/agent-metric-emission/...` passes |
| 3. `--run` pass-through | Forward the `-run` flag to `go test` | `e2ectl test --env val --suite ... --run TestAgentHeartbeat` runs one test |
| 4. Base-type mapping | `local` → `OnLocal`, `ec2-host` → `OnHost` | Both bases work with the right pattern |
| 5. Error UX | "env not ready → run these commands", "agent not installed → run install" | A missing-agent failure points at the fix, not at a stack trace |

## 7. What is deliberately deferred

- **CI mode** (`e2ectl test` as a CI runner that provisions on demand) — that's
  the vision's `ci` section, a separate feature.
- **Needs-based selection** — tests declaring "I need Kubernetes >= 1.29, agent
  installed, fakeintake" and the runner picking any matching environment.
- **Test filtering by capability** — `e2ectl test --env val --suite ...` when
  the test needs sudo but the environment is a container.
- **Parallel runs** — one test at a time per environment (the env is shared
  mutable state); parallelism comes from multiple named environments.
- **Kind support** — needs an `environments.Kubernetes` entry point pattern;
  same mechanism, just the Kubernetes type.
