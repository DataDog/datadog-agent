# autoscaling-fatal-startup-crashloop — Autoscaling startup failure is a clean fatal exit, not a partial run

**Type:** Reachability · **Assertion:** `Reachable` · **Priority:** P2 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): autoscaling-fatal-startup-crashloop

## Property

When autoscaling is enabled but the cluster name cannot be resolved or the remote-config client is unavailable, startup returns a fatal error and the process exits cleanly (pod crash-loops) rather than running in a half-initialized state.


## Invariant / assertion

`assert.Reachable(autoscaling_fatal_startup_path)`: under a startup fault that denies cluster name / remote config, the fatal return path (command.go:438-440, ~:565-607) is reached and the process exits. Reachable fits — the goal is to confirm the fatal path is actually taken (not silently swallowed) under fault, steering Antithesis to the startup-failure region.


## Antithesis angle

Most subsystems fail soft (log+continue), but autoscaling paths call return errors.New(...) → fatal. Inject apiserver latency/partition during startup so cluster name / remote config init fails; assert the process exits rather than serving a partially-initialized autoscaling controller.


## Why it matters

A half-initialized autoscaling controller could serve wrong/empty HPA data silently; a clean crash-loop is the intended, observable failure. Confirms fail-fast is honored under startup faults. (Lower Antithesis value — near-deterministic — but cheap and confirms the fail-fast contract.)


## Mechanism refinement (from open-question investigation)

No invalidation; confirms the Reachable invariant. Minor scope note: the fault-driven fatal path is the cluster-name one (command.go:437-440), since GetRFC1123CompliantClusterName returns '' promptly under a startup partition and caches it. The rcClient==nil fatals (:565/:579/:600) are config/fx-gated rather than transient-partition-driven (NewClient is local), so they are less relevant to the 'transient fault escalates to crash-loop' framing.


## Fault dependencies

- network partition / latency (leader<->apiserver) during startup (enabled by default)
- node termination to exercise restart (DISABLED by default)
- requires autoscaling enabled


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Reachable` at the fatal return; ensure no path swallows the error into a degraded run.


## Open questions (post-investigation)

None — all resolved (see Investigation Log).


### Investigation Log

#### Q1: Does GetRFC1123CompliantClusterName retry/block, or return '' immediately on transient apiserver failure?

Examined pkg/util/kubernetes/clustername/clustername.go:67-149. getClusterName does NOT retry: each ProviderCatalog func is called once, errors are Debug-logged and skipped (:101-105), node-label lookup errors are logged (:126-128); the result is cached via data.initDone=true (:145) which is set regardless of success. Concluded: returns '' promptly on a transient blip (bounded only by each provider call's own timeout), and once '' is cached it stays '' until ResetClusterName. So command.go:439 fatal is easily reached under a brief startup partition. RESOLVED.

#### Q2: Is rcClient==nil reachable in typical autoscaling deployments?

Examined command.go:477-513 and initializeRemoteConfigClient (:846). rcClient stays nil only when (a) rcEnabled&&isSet is false (RC disabled or rcService fx component not wired, :480) or (b) initializeRemoteConfigClient errors (:503). rcclient.NewClient is a LOCAL construction (no network call), so a transient apiserver partition does NOT null it. Concluded: rcClient==nil is config/fx-gated (RC disabled or RC-service unavailable), not driven by a transient apiserver blip; in a correctly-configured autoscaling deployment RC is required+enabled so nil arises mainly from misconfiguration or RC-service init failure. RESOLVED (narrows the fault-reachability angle: the cluster-name path, not rcClient, is the fault-driven fatal).

#### Q3: Does fxutil.OneShot returning an error always yield a non-zero exit code?

Examined pkg/util/fxutil/oneshot.go:22-60 (returns err from delayedCall.call() at :54-57), command.go:168 (RunE returns fxutil.OneShot(...)), and cmd/cluster-agent/main.go:28-31 (Execute() error → os.Exit(-1)). Concluded: a returned error propagates to os.Exit(-1) (exit 255, non-zero) → CrashLoopBackOff. Not swallowed. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `autoscaling-fatal-startup-crashloop`

## Mechanism (verified)

Fatal (return err → process exit) startup paths gated on autoscaling, in `cmd/cluster-agent/subcommands/start/command.go`:

- `:437-440` cluster name empty AND (`autoscaling.workload.enabled` OR `autoscaling.cluster.enabled`) → `return errors.New("Failed to start: autoscaling is enabled but no cluster name detected, exiting")`.
- `:563-566` `autoscaling.workload.enabled` and `rcClient == nil` → `return errors.New("Remote config is disabled or failed to initialize...")`.
- `:572-576` `StartWorkloadAutoscaling` error → `return fmt.Errorf(...)`.
- `:579-582` `autoscaling.cluster.enabled` and `rcClient == nil` → fatal return.
- `:584-586` `StartClusterAutoscaling` error → fatal return.
- `:590-596` `autoscaling.cluster.spot.enabled` StartSpotScheduling error → fatal return.
- `:600-608` `kubeactions.enabled` and `rcClient == nil` / Setup error → fatal return.

Contrast: soft-fail subsystems only log — admission (`:691` Errorf, continues), language detection (`:626` Errorf), appsec (`:633` Errorf), PAR (`:642` Errorf).

rcClient can be nil at these checks: `:502-504` `initializeRemoteConfigClient` error only logs and leaves `rcClient == nil` (declared at `:477`); it is non-nil only on the success branch (`:505-512`). RC init depends on the RC service and apiserver-backed identity, both fault-sensitive at startup.

clusterName can be empty: `clustername.GetRFC1123CompliantClusterName` (`:429`) falls back to hostname/auto-detection which routes through the apiserver; a startup partition can leave it empty.

The start command runs under `fxutil.OneShot(start, ...)`; a returned error propagates to a non-zero process exit, which under Kubernetes produces CrashLoopBackOff.

## Failure scenario

1. Deployment sets `autoscaling.workload.enabled=true` but relies on cluster-name auto-detection (no explicit `cluster_name`).
2. Antithesis partitions leader↔apiserver during startup; auto-detection returns "".
3. `command.go:439` returns fatal error → pod exits → CrashLoopBackOff.
4. Each restart re-triggers WaitForAPIClient block + leadership churn; if the partition persists, the DCA never comes up and the whole cluster loses DCA services, not just autoscaling.

## Key observations

- This is partly by design (fail-fast on a required dependency), so it is a Reachability property (document the crash-loop outcome), not necessarily a bug. Value: quantify how easily a transient startup fault escalates to a sustained crash-loop, and whether the fatal set is intentional vs over-broad.
- The cluster-name path is the most concerning because it is a transient/eventually-resolvable value being treated as fatal.

## Timing window

Escalation occurs entirely within a single startup pass; crash-loop persists as long as the underlying startup fault (partition / RC unavailability) persists.
