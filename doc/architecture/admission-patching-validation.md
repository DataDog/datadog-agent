---
title: Pod admission patch migration validation
---

The functional MVP is implemented from source baseline
`83b46e21baedd3c25542aea28a706114f18a8b36`. Local validation was completed on
October 5, 2026. All nine shared-wrapper Pod callers use explicit patch sessions;
the old typed Pod serialization and raw-to-typed diff runner and mutable-Pod
interfaces have been removed. SSI performance is deferred for this MVP.

`admission/patch` already contained Remote Config Deployment controllers on the
baseline. Those controllers moved to `admission/rcpatch`, with their implementation
and tests retained. The new Pod engine occupies `admission/patch` and depends only
on Kubernetes types and the pinned JSON Patch library. Controller network PATCH
infrastructure, CWS exec-options mutation, and spot DELETE handling retain their
separate paths.

The production writer audit classified the reachable assignments as follows:

| Path | Admission writes and ownership |
| --- | --- |
| Configuration | Ordered no-overwrite env insertions, fresh socket volumes/mounts, escaped annotations |
| Tags | Existing owner/label selection, sorted tag intent, no-overwrite env insertions |
| Autoscaling | Domain recommendation plan, individual annotation/resource keys, first GOMEMLIMIT occurrence; a private typed decision copy supports sequential recommendations and is never serialized as admission output |
| Spot | One tracker/placement decision, individual selectors/labels, appended toleration occurrences |
| SSI | Existing source/resolution/selection pipeline, env occurrence edits and joins, fresh injector templates, owned known fields of existing init recipes, explicit known volume-source replacement |
| AppSec | Shared config composition, fresh sidecars/init templates, exact argument edits, owned socket-access fields, existing ConfigMap/Gateway side effects |
| Agent sidecar | Fresh default templates, existing profile env/security/resource edits, exact provider volume/mount removals; TLS ConfigMap decisions execute once |
| CWS Pod | Fresh init template, first matching mount edit, explicit known volume-source replacement |
| NCCL | Fresh init template, no-overwrite env and volume/mount insertion helpers |

Fresh templates may be serialized as new entries. Existing recipes and profiles
describe owned known fields; removal of a selected known subtree also removes
its descendants. Surviving objects retain unrelated extension fields. The only
remaining `jsondiff.CompareJSON` in the admission mutation directory is the
out-of-scope CWS `PodExecOptions` path. There is no compatibility adapter that
infers admission writes from a mutated typed Pod, and no legacy fallback.

Existing mutation/controller tests on the unmodified baseline passed: 20 targets
were cache hits. The separate AppSec baseline run passed seven targets, with five
executed and two cache hits. After migration, the complete affected admission,
AppSec, workload autoscaling, and spot scheduler suite passed 37 targets. A final
focused run passed the sidecar and newly wired spot admission targets, giving
38 distinct passing targets. Existing AppSec webhook integration tests are included
in these local targets; they use test clients and are not deployed-cluster checks.

The verification commands were:

```sh
dda inv test --targets=./pkg/clusteragent/admission/...,./pkg/clusteragent/appsec/...,./pkg/clusteragent/autoscaling/workload,./pkg/clusteragent/autoscaling/cluster/spot --bazel-args='--test_output=errors --keep_going'
dda inv test --targets=./pkg/clusteragent/admission/mutate/spot,./pkg/clusteragent/admission/mutate/agent_sidecar --bazel-args='--test_output=errors'
dda inv cluster-agent.build
dda inv linter.go --targets=./pkg/clusteragent/admission/patch,./pkg/clusteragent/admission/patchtest,./pkg/clusteragent/admission/benchmarks,./pkg/clusteragent/admission/mutate,./pkg/clusteragent/admission/rcpatch,./pkg/clusteragent/admission/controllers/webhook,./pkg/clusteragent/appsec,./pkg/clusteragent/autoscaling/workload,./pkg/clusteragent/autoscaling/cluster/spot,./cmd/cluster-agent/subcommands/start --build=cluster-agent
git diff --check
```

The Cluster Agent build and scoped Go lint passed; lint reported zero issues.
Changed BUILD files passed buildifier in check mode. The generated DCA note
passed `reno --rel-notes-dir releasenotes-dca lint`. The required
`dda inv linter.releasenote` command exited successfully but skipped its PR gate
because no PR exists. The scoped release-note UID uniqueness check also passed.
The release-note skill's unsupported `--no-edit` flag was
corrected to the installed CLI's non-interactive default.

Existing typed feature fixtures now invoke real migrated planners through
`admission/patchtest`, apply the returned journal to their original input, and
retain known-field assertions. Pointer identity checks became value comparisons;
resource quantities use Kubernetes semantic equality. Expected wire patches now
describe intended insertions, and empty patches are `[]`. A fixture helper now
avoids an accidental duplicate of its autogenerated container when that name
is explicitly supplied; separate session cases verify ambiguous identities.

New coverage is concentrated in the session: optional absent/null/scalar parents,
pointer escaping, exact integers and decimals, detached snapshots, duplicate env
occurrences, stale handles, semantic quantities, exact removals, complete raw
normalization, deterministic ordering, request isolation, journal/application
equivalence, and batch failure atomicity. The container-stage case verifies
read-after-write across env, argument, mount, resource, normalization, and shifted
container edits, including rollback after a whole-Pod snapshot flush.

The focused preservation additions extend the existing configuration fixture with
`X=one, Y=$(X), X=two`, unrelated nested extension data, and exact large/decimal
numbers. Existing normalization cases use the real session; extension conflicts
are covered centrally. Wrapper cases cover annotation-only and normalization-only
results with `Injected=false`, empty journals, later feature failure, and ignored
helper errors. Sidecar and spot boundary checks establish invocation counts and
dry-run behavior without replacing their existing actual-feature policy tests.

The [benchmark harness](../../pkg/clusteragent/admission/benchmarks/README.md)
and [early diagnostic medians](admission-patching-benchmarks.csv) are retained.
They cover the requested local matrix on an Apple M4 Max, Darwin arm64, with three
100 ms samples per case. The prototype measurements precede final fixes and do
not establish final latency/allocation gates. SSI performance work is deferred;
staging p99 and timeout budgets have not been measured.

No live Kubernetes or new-e2e deployment was run. An internal-fork cluster context,
representative extension-bearing fixture, and its schema constraints were not
supplied. Direct raw patch application demonstrates local preservation; it does
not demonstrate persisted fork-server values. Fork persistence, a standard-cluster
admission check, and staging canary validation remain required before declaring
release/fork support. Use ordinary version canaries and rollback to the prior
Cluster Agent version; do not compare by invoking both live implementations or
retry a failed request through the legacy mutation algorithm.
