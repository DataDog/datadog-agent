# Health Platform — Developer Guide

## Issue identity fields

Every health issue has four identity fields. Follow these rules when adding a new issue module.

### `id`

- **Format**: kebab-case — lowercase letters, digits, and hyphens only
- **Scope**: unique per issue *instance* — used as the store map key
- **Variadic**: yes — callers may append a suffix to distinguish instances of the same type (e.g. `"ad-annotation:default/my-pod"`), or a hashed suffix when the distinguishing value isn't human-readable, as `invalidconfig` does with `"invalid-config:" + fnv64a(hostname + configPath)`. Prefer a 64-bit (or wider) digest over 32-bit for this — at 32 bits, fleets of ~10k+ distinct instances have a non-negligible birthday-collision chance, which can silently re-collapse the very instances the suffix exists to distinguish.
- **Examples**: `"invalid-config"`, `"rofs-permissions"`, `"docker-socket-permissions"`

### `issue_name` (`IssueName`)

- **Format**: Title Case — starts with an uppercase letter, followed by letters, digits, spaces, or hyphens
- **Scope**: stable per issue *type* — used as the registry lookup key and for aggregating all instances of the same issue type in the UI
- **Variadic**: no — must be a static constant, identical for every instance of the same issue type
- **Examples**: `"Read-Only Filesystem Error"`, `"Invalid Config"`, `"Autodiscovery Misconfiguration"`

> The registry panics at startup if `IssueName()` does not match `^[A-Z][a-zA-Z0-9 -]*$`.

### `issue_type` (`IssueType`)

- **Format**: `issue_name` lowercased with spaces replaced by underscores (hyphens are preserved) — e.g. `"Check Execution Failure"` → `"check_execution_failure"`
- **Scope**: stable per issue *type*, same scope as `issue_name` — a machine-friendly key for grouping/filtering in the backend
- **Caller-set, not derived**: the agent never computes this at runtime — that would duplicate logic the backend already owns. Each module exports a `const IssueType` next to `IssueName` (same lowercasing/underscore rule, written by hand) and sets it explicitly in `BuildIssue`, exactly like `IssueName`. `store.ReportIssue` passes it through unmodified.
- **Examples**: `"check_execution_failure"`, `"invalid_config"`, `"read-only_filesystem_error"`

### `title`

- **Format**: human-readable sentence, Title Case preferred
- **Scope**: specific to the issue *instance* — should surface the most actionable piece of context (affected entity, path, directory, check name, …)
- **Variadic**: yes — embed the instance-specific value directly in the string
- **Examples**: `"Docker log tailing disabled for '/var/lib/docker'"`, `"Autodiscovery Misconfiguration on 'default/my-pod'"`, `"Agent cannot write to: /var/lib/datadog-agent"`

> Static titles are acceptable only when there is genuinely no instance-specific value to embed (e.g. singleton issues like `"Admission Controller Unreachable"`).

## Issue lifecycle state

Issues have two canonical states defined in the `IssueState` proto enum:

| Value | Name | Meaning |
|---|---|---|
| `4` | `ISSUE_STATE_ACTIVE` | Issue is currently present |
| `3` | `ISSUE_STATE_RESOLVED` | Issue has been resolved |

The enum also retains three deprecated values for wire compatibility with older agents:
`UNSPECIFIED=0`, `NEW=1`, `ONGOING=2` — consumers must treat all three as equivalent to `ACTIVE`.
`RESOLVED=3` is unchanged from the original enum so agents that pre-date this simplification are handled transparently.

The state machine in the store (`store/impl/store.go`):
- Any `ReportIssue` call sets or keeps the issue `ACTIVE` and updates `LastSeen`.
- `ResolveIssue` / `ResolveAllIssues` transitions to `RESOLVED` and sets `ResolvedAt`.
- A resolved issue that is reported again resets to `ACTIVE` with a fresh `FirstSeen`.

On-disk state uses human-readable strings (`"active"`, `"resolved"`). The store accepts `"new"` and `"ongoing"` as legacy aliases for `"active"` when reading persistence files written by older agent versions (schema v2).

Persistence depends on the process and environment:

- Non-Kubernetes Agents use the local `issues.json` file.
- Kubernetes Agents with `health_platform.persist_on_kubernetes: true` also use that file, which requires a durable `run_path` volume.
- The long-running Kubernetes node Agent loads recent open issues from `https://api.<site>` when both `api_key` and `app_key` are configured. The application key must have `apm_read`; use a dedicated least-privileged key. Standard Agent proxy settings apply, but the metrics-intake `dd_url` override does not. This implementation is load-only; the existing Health Platform egress remains the write path.
- Otherwise on Kubernetes, Cluster Agents, Cluster Check Runners, one-shot Agent commands, node Agents without the required credentials, Agents using the local FIPS proxy, and Agents with TLS certificate verification disabled use no-op persistence.

## Cluster-wide issue collapse (`deployment_id`)

The backend dedups issues by `id` alone, ignoring hostname. By default this means every issue module
scopes its `id` per host (e.g. `fnv64a(hostname + templateIdentity)`), so a problem affecting `N`
node agents in a Kubernetes cluster produces `N` separate issues.

When a problem is actually caused by a template the cluster *distributed* to every node agent (a bad
cluster check, a cluster-distributed config file, a broken operator-rendered `datadog.yaml`), scoping
by hostname is wrong: it hides that the issue is shared and points remediation at one node instead of
the shared source. `comp/healthplatform/issueregistry/utils/selfident.SelfIdent` exists for this case:

- `SelfIdent.DeploymentID()` resolves the UID of the Kubernetes DaemonSet that owns the agent's own
  pod (via workloadmeta), cached for the process lifetime. Empty when not running under a DaemonSet
  (non-Kubernetes, or no DaemonSet owner).
- `SelfIdent.IssueDiscriminator()` returns `DeploymentID()`, or `""` when no DaemonSet owns this
  agent. It deliberately does *not* invent a per-host id, so that it behaves identically to the no-op
  build (see below).
- `issues.IssueDiscriminator(selfIdent, hostID)` is the function callers actually use: it takes
  `SelfIdent.IssueDiscriminator()` when non-empty, else `hostID`, else `os.Hostname()`. All agents
  owned by the same DaemonSet therefore compute the same discriminator, so `id`s built from it
  collapse into one backend issue instead of one per host; every other agent keeps per-host
  behavior. It tolerates a nil `selfIdent` (`ModuleDeps` is a plain struct).
- `healthplatformstore.Component.IssueDiscriminator(hostID)` (implemented in `store/impl`) delegates
  to that function with a process-shared `SelfIdent`, for Path-B reporters that hold the store
  component. Path-A modules that don't (`invalidconfig`, `invalidsysprobeconfig`) call it directly
  with their own `*selfident.SelfIdent` from `issues.ModuleDeps.SelfIdent` — a second,
  independently-cached instance is harmless since both resolve the same deterministic value.
- **`selfident` is behind the `kubeapiserver` build tag.** `selfident_noop.go` provides the same API
  returning `""` everywhere for flavors built without it — the iot and heroku agents,
  `cluster-agent-cloudfoundry`, and `serverless-init`. None of them can run as a Kubernetes
  DaemonSet, and every binary that both wires this bundle and can (`cmd/agent` base flavor,
  `cmd/cluster-agent`) has the tag. This keeps the resolver, and its retry loop, out of binaries
  where it could never succeed — `env.IsFeaturePresent(env.Kubernetes)` is not tag-gated, so a
  package-installed agent on a Kubernetes node would otherwise retry a lookup that cannot resolve
  without the kubelet workloadmeta collector. It also matters for the iot static quality gate, which
  has almost no headroom. If you add anything to `selfident`, keep both variants in sync.
- `cluster_id` (`SelfIdent.ClusterID()`) rides along in issue `Extra`/`Tags` for UI identification
  only — it is never part of the `id` itself. Unlike `DeploymentID()`, it resolves in a background
  goroutine and returns `""` until resolved: `clustername.GetClusterID()` usually requires a
  synchronous HTTP call to the Cluster Agent (`DD_ORCHESTRATOR_CLUSTER_ID` is not set by current
  Helm chart/Operator deployments), and `cluster_id` is best-effort metadata that must never block
  issue reporting on Cluster Agent availability.

**When adding or scoping a new module's `id`:** use `issues.IssueDiscriminator` instead of a bare
hostname/host ID whenever the module's failure mode can plausibly originate from a cluster-distributed
template (config validation, cluster check load/exec failures). Keep using a bare per-host id for
failures that are inherently host-local (e.g. filesystem permissions, local Docker socket access).

**Shared-resolution caveat:** because a collapsed `id` omits hostname, the first agent to recover
calls `ResolveIssue` with that same `id` and clears the issue for every other node still affected —
correct when the fix is genuinely shared, but it can flap if only some agents recover. Per-agent
affected-count is a backend follow-up, not handled by the agent today. Document this caveat at every
`ResolveIssue` call site for a collapsed issue.

## Current issues

| Package | `id` | `issue_name` | `issue_type` | `title` | `id` scoping |
|---|---|---|---|---|---|
| `admisconfig` (annotation) | set by caller | `Autodiscovery Annotation Misconfiguration` | `autodiscovery_annotation_misconfiguration` | `"<subtype> Misconfiguration on '<entityName>'"` | hostname-free (already collapses) |
| `admisconfig` (template) | set by caller | `Autodiscovery Template Resolution Error` | `autodiscovery_template_resolution_error` | `"Autodiscovery Template Resolution Error on '<entityName>'"` | hostname-free (already collapses) |
| `invalidconfig` | `invalid-config:<digest>` | `Invalid Config` | `invalid_config` | `"Datadog Agent Configuration Has <N> Schema Violation(s) in <filename>"` | `IssueDiscriminator` (cluster-collapsible) |
| `invalidsysprobeconfig` | `invalid-system-probe-config:<digest>` | `Invalid System-Probe Config` | `invalid_system-probe_config` | `"Datadog System-Probe Configuration Has <N> Schema Violation(s) in <filename>"` | `IssueDiscriminator` (cluster-collapsible) |
| `rofspermissions` | `rofs-permissions` | `Read-Only Filesystem Error` | `read-only_filesystem_error` | `"Agent cannot write to: <directories>"` | per-host (host-local failure) |
| `admissionprobe` | `admission-controller-connectivity-failure` | `Admission Controller Unreachable` | `admission_controller_unreachable` | `"Admission Controller Unreachable"` | singleton |
| `dockerpermissions` | `docker-socket-permissions` | `Docker File Tailing Disabled` | `docker_file_tailing_disabled` | `"Docker log tailing disabled for '<dockerDir>'"` | per-host (host-local failure) |

## Adding a new issue module

1. Pick an `id`: kebab-case, unique across all modules.
2. Pick an `issue_name`: Title Case, describes the *class* of issue (not a specific instance).
3. Derive `issue_type` by hand from `issue_name`: lowercase it and replace spaces with underscores (keep hyphens as-is).
4. Export all three as constants in `module.go`:
   ```go
   const (
       IssueName = "My New Issue"          // Title Case, stable
       IssueType = "my_new_issue"          // IssueName lowercased, spaces -> underscores
       IssueID   = "my-new-issue"          // kebab-case, unique
   )
   ```
5. In `BuildIssue`, set both `IssueName` and `IssueType` to the fixed constants, and set `Title` to a string that embeds the instance-specific value from `context`.
6. Register the module via `issues.RegisterModuleFactory(NewModule)` in an `init()` function.
