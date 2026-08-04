# Antithesis Harness — Datadog Cluster Agent

Status doc for the Antithesis effort targeting the **Cluster Agent (DCA)**. For
the full research (property catalog, evidence, design rationale) see
`scratchbook/`. This file is the "what actually exists and works right now" summary.

## What this is, in one paragraph

Antithesis is a deterministic fault-injection platform: it runs the whole system
(as Docker containers) inside a hypervisor that can partition/kill/slow containers
and control thread scheduling, replaying any interesting failure exactly. We're
building a harness so it can run the real Cluster Agent against a real (if
minimal) Kubernetes control plane, inject faults, and check the 37 correctness
properties in `scratchbook/property-catalog.md` — things like "at most one
replica is ever the active leader" and "a cluster check never runs on two nodes
at once."

## Pipeline

```
antithesis-research  →  antithesis-setup (we are here)  →  antithesis-workload
   (property catalog)      (build images, wire the SDK)      (write the actual
                                                                fault scenarios)
```

## What's running (`antithesis/config/docker-compose.yaml`)

| Container | What it is | Why |
|---|---|---|
| `kube-init` | one-shot: generates a self-signed CA, a TLS cert for the fake apiserver, and a static bearer token | Kubernetes normally mints these automatically via its controller-manager; we don't have one, so this stands in for it |
| `etcd` | real etcd (official image) | the apiserver's database |
| `kube-apiserver` | real kube-apiserver binary, repackaged onto Alpine | a **bare** control plane — no scheduler, no controller-manager, no real nodes. Just enough for the DCA to read/write Leases, ConfigMaps, Endpoints — everything the property catalog actually needs |
| `dca-1`, `dca-2` | the real, unmodified Cluster Agent binary | two replicas so leader election has something to actually contend over |
| `workload` | placeholder (does nothing yet) | `antithesis-workload` will replace this with code that impersonates node agents and drives failure scenarios |

**Why kube-apiserver is "bare":** a real cluster auto-schedules pods, manages
nodes, and issues each pod a signed identity token. None of that is needed to
test the DCA — it only touches a handful of API object types (Lease, ConfigMap,
Endpoints/EndpointSlice, eventually CRDs), all of which just live in etcd via the
apiserver. `kube-init`'s job is to fake the one piece we do need: a way for the
DCA to authenticate. Both replicas share one static bearer token; `kube-apiserver`
runs `--authorization-mode=AlwaysAllow`, so any authenticated caller can do
anything — there's no real RBAC being tested here, on purpose.

## The Dockerfiles we built and why they're not the release ones

- `antithesis/kube-apiserver.Dockerfile` — the official `registry.k8s.io/kube-apiserver`
  image is distroless (no shell). Docker Compose healthchecks need to exec a
  command *inside* the container, so we copy the same binary onto Alpine.
- `antithesis/dca.Dockerfile` — `Dockerfiles/cluster-agent/Dockerfile` (the real
  release Dockerfile) only assembles a runtime image from binaries built
  *elsewhere* by CI; it has no from-source build stage. Ours builds from source.
- `antithesis/kube-init.Dockerfile` — bakes `openssl` into the image at build
  time (see "Real bugs" below — installing it at container start needs internet
  access, which Antithesis runs don't have).

## How the DCA is built (and what we tried that didn't work)

**What works:** a plain `CGO_ENABLED=1 go build -tags "clusterchecks,..." ./cmd/cluster-agent`
— no `dda inv`, no Bazel. (`dda inv cluster-agent.build`'s own compile step is
itself just a `go build` under the hood; Bazel is only used elsewhere for
dependency bookkeeping.) The SDK is linked as a real dependency
(`go.mod`/`go.sum`), and one bootstrap call —
`assert.Reachable("cluster-agent start() entered", nil)` in
`cmd/cluster-agent/subcommands/start/command.go` — proves the SDK reports
correctly at runtime.

**What we tried and dropped:** running `antithesis-go-instrumentor` (Antithesis's
coverage-instrumentation + static-assertion-cataloging tool) over the whole
repo. It requires the *entire* ~11,400-file module to load and type-check
cleanly, and a handful of unrelated packages (an eBPF test helper, a stale
benchmark, some build-tag-gated test utilities — nothing the Cluster Agent
imports) fail that load, which kills cataloging for the whole thing. We found an
existing internal precedent — `blt/antithesis-harness` (PR #51515), an Antithesis
effort on the logs-agent in this same repo — that hit the identical wall and
made the same call: skip the instrumentor, link the SDK directly, hand-write
`assert.*` calls. **What we lose:** coverage-guided fuzzing feedback and a
pre-run "this assertion was never reached" report. **What we keep:** every
assertion still fires and reports normally when hit — cataloging is a separate,
additive layer, not a prerequisite. Full writeup:
`scratchbook/deployment-topology.md` → "Instrumentation decision."

## Verified so far

Built both images, brought up the full 6-container stack, and confirmed real
(not mocked) behavior:

- `kube-apiserver` and `etcd` pass real Docker healthchecks (authenticated
  `/healthz` calls, `etcdctl endpoint health`).
- `dca-1` and `dca-2` both talk to the fake apiserver, both see
  `Connected to kubernetes apiserver, version v1.31.1`.
- **Real single-leader election**: `dca-1` acquires the `coordination.k8s.io`
  Lease and logs `Currently Leader: true`; `dca-2` correctly observes
  `Currently Leader: false. Leader identity: "cluster-agent-dca-1"`. This is the
  actual, unmodified `pkg/util/kubernetes/apiserver/leaderelection` code running
  against our fake control plane — exactly the code path the catalog's P0
  property (`dispatch-implies-lease-holder`) targets.
- `LeaderLeaseDuration: 15s` confirms the harness's short-lease commitment
  (needed so leadership-flap properties are reachable without clock-skew/node-
  termination faults, both commonly disabled by default on Antithesis tenants)
  is actually taking effect.

`snouty validate antithesis/config` **passes**: "Setup-complete event detected."
/ "Setup validation successful." (0 test commands found is expected — the
workload/test-template are still placeholders, `antithesis-workload`'s job.)

## Real bugs caught and fixed along the way

- **Two DCA replicas silently using different leader-election lock objects.**
  The `docker-compose.yaml` env blocks had nearly-identical but not-quite-identical
  text, so a config fix applied via find-and-replace landed on only one of
  them — `dca-1` used the Lease object, `dca-2` silently kept the default
  (ConfigMap) object, so they never actually contended with each other and
  **both** claimed to be leader. Fixed by replacing the duplicated blocks with a
  shared YAML anchor (`x-dca-env`) so the two replicas structurally cannot drift
  again.
- **`kube-init` violated hermetic execution.** It ran `apk add openssl` at
  container *start* — Antithesis runs have no internet access once sealed, so
  this failed under `snouty validate`'s isolated network (worked fine under a
  plain `docker compose up`, which still has normal internet access, masking the
  bug). Fixed by baking `openssl` into a purpose-built image
  (`antithesis/kube-init.Dockerfile`) at *build* time instead.
- **`kube-apiserver` couldn't determine its own address.** It auto-detects
  `--advertise-address` from the host's default route, which doesn't exist under
  `snouty`'s isolated network (`"no default routes found in /proc/net/route"`).
  Fixed by resolving `$(hostname -i)` explicitly instead of relying on
  auto-detection.
- **`setup-complete.sh` was never actually invoked.** Copying the script into an
  image doesn't run it. Wired it into the `workload` placeholder's startup
  command — and separately found the script's `[[ ]]` test syntax silently no-ops
  under `sh`/dash (must be invoked with `bash`).
- **(Local-validation-only, not a harness bug) `snouty`'s scratch directory landed
  under macOS's `/var/folders`, which this machine's Colima VM doesn't share** —
  confirmed with a plain `docker run -v /var/folders/.../x:/tmp/y ...` outside
  snouty/compose entirely, which reproduced the identical silent failure, and a
  control test to a `$HOME` path, which worked. This doesn't affect the real
  Antithesis environment (no macOS/Colima boundary exists there) — workaround for
  local runs: `TMPDIR="$HOME/.cache/snouty-tmp" snouty validate antithesis/config`.
- **First real `snouty launch` failed: `etcd`/`debian` images referenced their
  original public registries** (`registry.k8s.io`, `docker.io`). `snouty` only
  pushes images it doesn't already consider "in a registry" — it assumed
  Antithesis could pull these directly, but Antithesis's provisioning
  environment has no general internet access (the same hermetic-execution
  constraint as `kube-init`, above). Fixed by retagging both under plain local
  tags (`cluster-agent-etcd:antithesis`, `cluster-agent-workload-base:antithesis`)
  so `snouty` mirrors them into `ANTITHESIS_REPOSITORY` like every other image.
  **Every image in the compose file needs to be reachable from our own
  registry, not just the ones we build.**
- **Plain `docker pull --platform linux/amd64` silently ignored the platform
  flag on this Colima setup** — even a fresh pull after `docker rmi` still
  resolved to the host's native arm64, causing `snouty launch` to fail with
  `'docker image inspect ...' returned empty architecture`. Fixed by resolving
  through `buildx` instead, which correctly honors `--platform`:
  `printf 'FROM <image>\n' | docker buildx build --platform linux/amd64 --load -t <tag> -f - .`
  Full details and the exact commands: `~/sandbox/antithesis-cluster-agent-run-2026-08-04/NOTES.md`.

## What's not done yet

- **`snouty launch` accepted a second run** (`c46848efc4156ead5a6ff7a8a7b3d10a-58-11`,
  10 min, webhook `basic_test`) after fixing the two image-mirroring bugs above.
  The first attempt (`725488c3bbf9b06ad02e8bca4d197a16-58-11`) was also
  *accepted* (202) but then failed Antithesis's own internal setup validation
  once provisioned — `snouty launch` returning 202 only means the request was
  queued, not that the run will actually come up. Still need to check this
  second run's outcome via `snouty runs --json show <run_id>` once it
  provisions.
- `workload` is still a no-op placeholder — `antithesis-workload`'s job.
- The "Deferred decisions" list in `scratchbook/deployment-topology.md` (webhook
  Service selector, StatefulSet vs Deployment, whether to request clock-skew/
  node-termination faults from the tenant, etc.) is still open.
- No CLC-runner, external-metrics backend, remote-config, or admission-webhook-client
  stub containers yet — add per `scratchbook/deployment-topology.md`'s
  "Conditional" container list, only when a property needs one.

## Running it locally

```bash
cd antithesis/config
docker compose build kube-apiserver dca-1   # dca-2 reuses dca-1's image tag
docker compose up -d
docker compose logs dca-1 dca-2 | grep -i leader
docker compose down -v
```

On Apple Silicon, `docker compose build` runs under QEMU emulation (the compose
file declares `platform: linux/amd64` for real Antithesis submission) — expect
it to take a while (~30 min cold) even though the build itself is simple. For
faster local iteration, `docker build --platform linux/arm64 -f antithesis/dca.Dockerfile -t cluster-agent-dca:antithesis-test .`
bypasses compose's platform pin.

Running `snouty validate antithesis/config` **on macOS with Colima**: set
`TMPDIR` to somewhere under `$HOME` first (Colima doesn't share macOS's system
temp dir, where `snouty` creates its scratch directory, into its VM by default):

```bash
mkdir -p "$HOME/.cache/snouty-tmp"
TMPDIR="$HOME/.cache/snouty-tmp" snouty validate antithesis/config
```
