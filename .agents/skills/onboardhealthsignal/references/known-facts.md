# Verified facts (read 2026-10-20; re-verify when stale)

Sources: datadog-agent main 21522ee, dd-source main d715db2 (worker.ts last touched 6c7ca71), agent-payload a0a2f27, live Mosaic read. AGENTS.md at the target worktree SHA overrides this file.

## Identity
- IssueName: Title Case, `^[A-Z][a-zA-Z0-9 -]*$`, identical for every instance of the type.
- IssueType: IssueName lowercased, spaces to `_`, hyphens kept. A fixed const set in BuildIssue; the agent never derives it at runtime. Path A modules also implement `IssueType() string`.
- IssueID: kebab-case base const plus an instance suffix. Scope per host (hash of `issues.IssueDiscriminator(selfIdent, hostname)`) or per entity. A bare constant collapses every host in an org into one backend issue if the backend keys on org plus ID; that keying is UNVERIFIED (the repo has two contradictory comments, one saying one row per issue type), so ask the Agent Health team.
- BuildIssue never sets the issue ID; Path A runner does; Path B reporters must set it before `store.ReportIssue` (empty is rejected).

## Worker drop rules (`agthealth-worker` worker.ts:536-562)
Non-resolved reports need non-empty issueName, category, host hostname, issueType (agents that send it), title, description, and severity (not 0/UNSPECIFIED). Any miss drops the report from all sinks (`missing_required_fields`). Resolved reports skip title/description/severity.
- Severity wire: proto enum LOW=1, MEDIUM=2, HIGH=3; worker emits lowercase string (low/medium/high/unknown). Never gap-filled by the catalog.
- State: the worker emits `STATE_RESOLVED` for RESOLVED and `STATE_UNKNOWN` for everything else; it never emits OPEN. The layer that shows "open" downstream is unconfirmed; do not assert OPEN.
- Per-org flag `agthealth-worker-process`: staging all orgs, prod a short org list plus several datacenters.
- `agthealth-worker-drop-issue`: drops by issue_name, category, or agent_version. As of 2026-08-31 it drops categories `configuration` and `autodiscovery` in prod. Check live with the check-mosaic-drift skill.
- `agthealth-catalog-resolver`: strict `true` only (default off). Staging on for all orgs; prod on for 7 org IDs (2026-10-20 read).

## Recommendations API
`insight_types` accepts `agent_health_insight` (canonical), `agenthealth`, `agent_health`. Filter by `scope.agent_health.issue_name` (exact IssueName). Without `insight_types` the default query excludes agent-health results.

## Agent side
- Path A layout: `comp/healthplatform/issues/<pkg>/{module.go,issue.go,check.go,check_noop.go?,BUILD.bazel}` plus a blank import in `comp/healthplatform/bundle.go`. Missing import: no error, only a log `no issue template found`.
- Path B layout: `<type>_issue.go` plus BUILD.bazel; no module.go, no init, no bundle import.
- Check funcs return `([]IssueReport, error)`; `(nil, nil)` means healthy so the scheduler resolves; a non-nil error prevents resolution. A periodic check `Source` must be unique among periodic checks (a duplicate is only logged and the check never runs).
- `check_noop.go` with the negated build tag is required when `check.go` has a build constraint. A module with no detection returns nil from both `BuiltIn*` methods (AGENTS.md); a registered check that cannot report only costs a goroutine and ticker in every agent and cluster agent.
- Remediation: Steps contiguous 1-indexed, step 1 a diagnostic command, no PII or secrets in strings. Extra keys snake_case on the wire (context keys may differ).
- Release note `releasenotes/notes/*.yaml`; cluster agent uses `releasenotes-dca`. While detection is a TODO use the `changelog/no-changelog` label instead of a note.
- Whether the root `comp/healthplatform` test target carries a feature build tag depends on the runner and SHA (one `dda inv test` run compiled a tag-gated check as its noop, another logged the tag): read the tags in the log before relying on either.
- `env.IsFeaturePresent` panics before feature detection has run (tests call `env.SetFeatures`); features are detected once at start. Ready-made features live in `pkg/config/env` (grep them) before you write a gate of your own.
- Tests that use unix sockets under a build tag that is also in Windows agent builds need a `linux` constraint (the Windows client uses a named pipe).
- The scheduler runs each periodic check once immediately at start, then on its interval; a bundle test that enables the gate through config and environment reaches the issue on the first tick.
- In-process pattern: `comp/healthplatform/bundle_test.go` and `invalidconfig_integration_test.go` (`fakeintakeserver.NewServer`, `fxutil.Test`, `config.NewMock` plus `SetInTest` for api_key, dd_url, health_platform.enabled, forwarder.interval, run_path, then poll `GetAgentHealth()`). Build tag `test`.

## Tools on the dev machine
Present: go (PATH recipe `/opt/homebrew/opt/go@1.25/bin:$HOME/go/bin`), bazel, dda, node, op. Missing at last check: docker, mise, ddr (the user runs conductor deploys). `npm` may be an alias for a supply-chain wrapper whose environment is missing: use `node`, `turbo` and `node_modules/.bin/*`, and never run an install; the user runs installs. Gazelle takes 1 to 9 minutes depending on cache warmth and contention; a shared bazel disk cache (`preflight.sh cache`) and stopping idle servers (`preflight.sh bazel-shutdown-idle`) are the main speed levers.

## Verified build facts (2026-10-20, worktree at 51d934317f)
- A fresh worktree needs `dda inv schema.codegen` before any package importing `pkg/config/setup` compiles (generated `all_settings.go`, `generated.go`).
- `hostnamemock.NewMock` exists only under build tag `test` and takes `hostnamemock.MockHostname`.
- A test that imports the issue package registers the module via `init()` and therefore cannot detect a missing `bundle.go` import.
- Most existing modules have no registration-level test at last audit (for example admissionprobe, docker, rofspermissions); check 17 fails for them by design.
- `rofspermissions` reports a bare `IssueID` constant (check 3 flags it); likely the same cross-host collapse fixed for docker.

## Ownership and dependency facts (verified at agent main 51d934317f, 2026-10-20)
- `.github/CODEOWNERS` line 480: `/comp/healthplatform @DataDog/fleet-remediation` covers every subdirectory. Per-module co-owner lines exist only where a feature team co-owns (lines 669-672 at that SHA, for example `ad-misconfiguration`, `gpuenvironment`, `rofspermissions`). Most feature-owned sibling signals also carry a per-module line, so recommend a co-owner line when I1 points into a feature team's code (ask when interactive; open question in inputs mode). `AGENTS.md` notes `CODEOWNERS` is partly generated (a components block), so edit outside it.
- The cluster agent also starts the bundle: `cmd/cluster-agent/subcommands/start/command.go:220` calls `healthplatform.Bundle()`. Anything an issue module imports without a build tag is linked into the cluster agent. Heavy or platform-specific imports are fine only in a build-tag-gated file with a `check_noop.go` stub (the cluster agent and other builds then compile without them); measure with `scripts/deps_delta.sh` (`go list -deps` of the cluster agent binary versus the new package). `pkg/config/env` and stdlib packages such as `expvar` are already linked. Share state through a small leaf package or inject a provider via `issues.ModuleDeps` (fields: Config, SysProbeConfig, Hostname, SelfIdent).
- The scheduler resolves every issue ID a periodic check stops returning, and at startup it seeds each periodic check with the persisted active IDs for its issue name; on the first run it resolves every seeded ID the check does not return. Hysteresis above 1 therefore makes a persisted issue vanish for one interval after a restart unless the check returns an error for a found but unconfirmed instance (stateless fix; a first-run flag does not work). Persistence is a no-op on Kubernetes unless `health_platform.persist_on_kubernetes` is set, so the seed is empty there by default. A signal that must clear by itself fits Path A periodic; Path B reporters must resolve explicitly, including ids persisted under another id and everything when the owner is disabled.
- Path B facts: `id_scope` is a Path A renderer option and has no effect on Path B (the owner adapter chooses the id). `store.ReportIssue` does a synchronous disk write plus a metric increment per call, so an owner should notify on transitions (and on its first evaluation after start), not every cycle. The store offers `IssueDiscriminator` (collapses per DaemonSet UID); it is right for per-workload conditions and wrong for per-node conditions that recover independently. A persisted active issue is in the store's by-name map but not its live map after a restart, and `ResolveIssue` handles that persisted-only case, so a first evaluation that finds the condition gone resolves it. Other binaries that link an owner component typically have no store provided: the optional field is nil there.
- `dda inv test` flags differ by SHA: current main (verified 51d934317f) has `--targets`, `--only-modified-packages` (non-recursive, `//pkg:all`; ignores brand-new untracked packages), `--test-args='-test.run=^Name$ -test.v'`, `--bazel-args='--test_output=all'`, `--race`, `--module`; older SHAs lack `--test-args` and `--bazel-args` and use `--test-run-name='^Name$' -v` (and may run gotestsum with feature tags rather than Bazel). Probe `dda inv test --help`. Baseline failures at some SHAs ("no embedded schema") are SHA-specific; use `scripts/baseline_check.sh`.
- Linter: `dda inv linter.go --module=. -v --targets=<pkg>,./comp/healthplatform`; `perfsprint` rejects `fmt.Sprintf("...%s", x)` with a single `%s` (use concatenation); `revive package-comments` needs a package comment in at least one non-test file.
- Catalog tooling: `domains/evp-workers` is an npm workspace root (workspaces `libs/**/*`, `worker-libs/**/*`, `apps/*`, `tools/*`, `docs`); prettier 3.2.5 is a root devDependency; the worker's `npm test` runs `event-workers test` (jest preset from the testkit). A sparse checkout of only the app directory cannot run them.
