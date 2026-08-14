# Antithesis Harness — Datadog Cluster Agent

A fault-injection harness that runs the real Cluster Agent (DCA) against a
minimal Kubernetes control plane inside Antithesis, so deterministic faults
(partitions, kills, scheduling control) can probe the 37 correctness properties
in `scratchbook/property-catalog.md` — "at most one replica is the active
leader," "a cluster check never runs on two nodes at once," and the rest.

```
antithesis-research  →  antithesis-setup (we are here)  →  antithesis-workload
   (property catalog)      (build images, wire the SDK)      (write the fault scenarios)
```

## Topology

`config/docker-compose.yaml` brings up six containers:

| Container | Role |
|---|---|
| `kube-init` | one-shot: generates a self-signed CA, an apiserver serving cert, and a static bearer token shared by both DCA replicas |
| `etcd` | the apiserver's database |
| `kube-apiserver` | a **bare** control plane — no scheduler, no controller-manager, no nodes. The DCA only touches Leases, ConfigMaps, Endpoints/EndpointSlices; all of that lives in etcd via the apiserver. `--authorization-mode=AlwaysAllow` + a static token file stands in for the identity machinery a real cluster provides |
| `dca-1`, `dca-2` | the real Cluster Agent, built from source, two replicas so leader election has something to contend over |
| `workload` | placeholder — `antithesis-workload` replaces it with code that impersonates node agents and drives the fault scenarios |

The apiserver is its own container (not bundled into a `k3s`/`kind` image)
because the catalog's highest-value fault is partitioning a DCA replica *from
the apiserver* — Antithesis faults are container-level, so anything we want to
fault independently must be its own container.

## Building the DCA image

`antithesis/dca.Dockerfile` builds the cluster agent from source with a plain
`CGO_ENABLED=1 go build -tags "clusterchecks,...,antithesis"` — no `dda inv`,
no Bazel. The release `Dockerfiles/cluster-agent/Dockerfile` only assembles a
runtime image from CI-built artifacts; it has no from-source stage.

The Antithesis SDK is a real `go.mod` dependency, but the import is
**build-tag-gated** so it never enters a normal cluster-agent build:

- `cmd/cluster-agent/subcommands/start/antithesis_assert.go` (`//go:build antithesis`)
  imports the SDK and defines `emitBootstrap()`, which fires
  `assert.Reachable("cluster-agent start() entered", nil)`.
- `antithesis_assert_noop.go` (`//go:build !antithesis`) provides an empty
  stub, so a build without the tag links **zero** SDK code (`go tool nm`
  confirms no `antithesishq` symbols).
- Only `dca.Dockerfile` passes `-tags ...,antithesis`. The `antithesis` tag is
  registered in `tasks/build_tags.bzl` `ALL_TAGS` but deliberately not in
  `CLUSTER_AGENT_TAGS`.

We skipped Antithesis's `antithesis-go-instrumentor` (coverage instrumentation
+ static assertion cataloging). It requires the entire ~11,400-file module to
type-check cleanly, and a handful of unrelated packages fail that load, which
kills cataloging for the whole binary. `blt/antithesis-harness` (PR #51515,
logs-agent) hit the identical wall and made the same call: link the SDK
directly, hand-write `assert.*`. The cost is coverage-guided fuzzing feedback
and a pre-run "assertion never reached" report; every assertion still fires
and reports normally at runtime. Full writeup: `scratchbook/deployment-topology.md`
→ "Instrumentation decision."

## Status

The harness is live end-to-end through the setup phase. `snouty validate
antithesis/config` passes, and a real `snouty launch` (run
`c0ff9806864e6266dc0cf1349b85db2a-58-11`, 2026-08-05) completed successfully:
all six containers came up, `setup_complete` fired, and the bootstrap assertion
reported 3,555 times — the SDK is genuinely linked and reporting.

Verified behavior, all against the real (unmocked) DCA and apiserver:

- `kube-apiserver` and `etcd` pass real Docker healthchecks.
- Both DCA replicas connect and see `Connected to kubernetes apiserver, version v1.31.1`.
- **Real single-leader election**: `dca-1` acquires the `coordination.k8s.io`
  Lease and logs `Currently Leader: true`; `dca-2` observes `Currently Leader:
  false. Leader identity: "cluster-agent-dca-1"`. This is the unmodified
  `pkg/util/kubernetes/apiserver/leaderelection` code — the path the catalog's
  P0 property `dispatch-implies-lease-holder` targets.
- `LeaderLeaseDuration: 15s` confirms the short-lease setting that makes
  leadership-flap properties reachable without clock-skew/node-termination
  faults (both commonly disabled by default).

### First finding from fault injection

The same run surfaced a real defect, not a harness bug: **`dca-1`/`dca-2` exit
with code 255 on transient apiserver/etcd unavailability instead of retrying.**
At vtime 51.88s an injected network fault timed out `dca-1`'s lease-update RPC
(`leaderelection.go:188`: `Could not initialize the Leader Election process:
rpc error: code = Unavailable ... read: connection timed out`). The next log
line reads `Error: temporary failure in leaderElection, will retry later` — but
the container exits immediately after, rather than retrying. `dca-2` hits the
identical pattern at vtime 30.95s. Neither service has a `restart:` policy, so
a replica that dies this way never comes back. Candidate location:
`pkg/util/kubernetes/apiserver/leaderelection/leaderelection.go` around
lines 105/188 — an init-time RPC error treated as fatal despite the retry
message. (Report: https://datadog.antithesis.com/report/GG_tu-9m_w8y8S3Bkr9ETQO0/2pH0GfQLyfv7QjhZAnpvvW8eJBbtYhy9B3vLRCsVUvg.html)

## What's left

- `workload` is still a no-op placeholder — `antithesis-workload` writes the
  real test commands (impersonate node agents, seed AD configs, manage the DCA
  Service + EndpointSlice, emit the catalog's assertions).
- Deferred harness decisions (webhook Service selector, StatefulSet vs
  Deployment, whether to request clock-skew/node-termination faults) are open
  in `scratchbook/deployment-topology.md`.
- No conditional stub containers yet (clc-runner, external-metrics backend,
  remote-config, admission-webhook-client) — add per the topology's
  "Conditional" list only when a property needs one.

## Running locally

```bash
cd antithesis/config
docker compose build kube-apiserver dca-1   # dca-2 reuses dca-1's image tag
docker compose up -d
docker compose logs dca-1 dca-2 | grep -i leader
docker compose down -v
```

On Apple Silicon, `docker compose build` runs under QEMU emulation (the
compose file pins `platform: linux/amd64` for real submission) — expect ~30
min cold. For faster iteration, bypass the platform pin:

```bash
docker build --platform linux/arm64 -f antithesis/dca.Dockerfile -t cluster-agent-dca:antithesis-test .
```

`snouty validate` on macOS with Colima: set `TMPDIR` under `$HOME` first
(Colima doesn't share the system temp dir, where `snouty` creates its scratch
directory, into its VM):

```bash
mkdir -p "$HOME/.cache/snouty-tmp"
TMPDIR="$HOME/.cache/snouty-tmp" snouty validate antithesis/config
```
