# Container image hidden bytes: manual QA

This validates the **Agent payload**, not a backend aggregate or UI. It uses
containerd overlayfs snapshots, not image tarballs, and checks exact values received
at fakeintake's `/api/v2/contimage` endpoint. A missing field is not zero.

Use a dedicated **ARM64** Docker/minikube environment. These wrappers use an ARM64
Agent base pinned by digest; on another architecture select a compatible pinned
base and rebuild all binaries for that architecture. Allow roughly 6 GiB RAM,
4 CPUs and sufficient free disk for the build and loaded images. Do not prune
unrelated images or reset another cluster to make room.

All commands below run from the repository root. Prerequisites: `dda`, Docker,
minikube, `jq`, and the companion agent-payload revision providing `hidden_bytes`.
Build the Agent and fakeintake against that same revision; an older fakeintake
client cannot display the new field. The assets deliberately avoid production
credentials, Helm, or an external intake. Fakeintake forwarding is explicitly off.

## 1. Build and start the isolated environment

```sh
dda inv agent.build --exclude-rtloader --build-exclude=python,systemd
dda inv fakeintake.build

docker build -f test/integration/container_image_hidden_bytes/Dockerfile.agent \
  -t localhost/hidden-bytes/agent:qa .
docker build -f test/integration/container_image_hidden_bytes/Dockerfile.fakeintake \
  -t localhost/hidden-bytes/fakeintake:qa .
for scenario in seeded deleted replaced combined replaced-deleted same-step; do
  docker build -f test/integration/container_image_hidden_bytes/Dockerfile.fixtures \
    --target "$scenario" -t "localhost/hidden-bytes/${scenario}:qa" .
done

minikube start -p hidden-bytes-validation --driver=docker \
  --container-runtime=containerd --kubernetes-version=v1.35.1 --memory=6144 --cpus=4 --keep-context
for image in agent fakeintake seeded deleted replaced combined replaced-deleted same-step; do
  minikube -p hidden-bytes-validation image load "localhost/hidden-bytes/${image}:qa"
done
```

Use `minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation`
for every Kubernetes command; this downloads the matching kubectl instead of using
an incompatible host version. Inspect the runtime before deployment:

```sh
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation get nodes -o wide
minikube -p hidden-bytes-validation ssh -- 'sudo ctr plugins ls | grep snapshotter'
minikube -p hidden-bytes-validation ssh -- 'sudo test -d /var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots'
```

The expected layout is `/var/lib/containerd` with socket
`/var/run/containerd/containerd.sock`. Adjust both host paths if the node uses
different locations. Do not silently scan a different runtime or snapshotter.

The initial implementation omits results for hardlinked files, unsupported
snapshotters, remapped user namespaces, metacopy/redirect metadata, unavailable
snapshots, or scans exceeding their limits. It requires `CAP_SYS_ADMIN` in the
initial Linux user namespace. These limitations apply to the whole image:
partial counts are never published.

```sh
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  apply -f test/integration/container_image_hidden_bytes/namespace.yaml
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  apply -f test/integration/container_image_hidden_bytes/stack.yaml
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  apply -f test/integration/container_image_hidden_bytes/fixtures.yaml
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  -n hidden-bytes-validation rollout status deployment/agent --timeout=180s
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  -n hidden-bytes-validation wait --for=condition=Ready pod/fixtures --timeout=180s
```

**QA-only permissions:** the core Agent container gets a read-only mount of the
node's containerd storage at `/host/var/lib/containerd`, with `HOST_ROOT=/host`.
It also gets the runtime socket and `CAP_SYS_ADMIN` so native overlayfs metadata
can be inspected. Read-only does not hide file contents, and a mounted runtime
socket grants runtime API access even when its mount is read-only. This is an
isolated test configuration, not a production permission recommendation.

## 2. Assert actual received metrics

In another terminal, keep this port-forward running:

```sh
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  -n hidden-bytes-validation port-forward service/fakeintake 18080:8080
```

```sh
test/fakeintake/build/fakeintakectl --url http://127.0.0.1:18080 flush
bash test/integration/container_image_hidden_bytes/assert_payloads.sh
```

Flush only after the Agent has settled and any startup diagnostics have finished.
Connectivity diagnosis can POST a JSON `{}` probe to the same `/api/v2/contimage`
route. Fakeintake's typed image parser expects protobuf, so a retained probe causes
`proto: cannot parse invalid wire-format data`, even when the actual image payloads
are correct. Avoid `agent diagnose` during assertions; if a probe appears, finish
diagnostics, flush, and retry. Periodic refresh will send the image payloads again.

The script resolves the immutable config ID from Docker's export manifest and uses
ordered RootFS DiffIDs to select
the exact image, rather than confusing a tag or compressed layer digest with a
DiffID. It verifies every filesystem layer, including explicit zero values, and
waits up to two minutes per image for asynchronously collected results.

| Fixture | Seed layer hidden bytes | Later replacement layer hidden bytes | Image total for QA only |
| --- | ---: | ---: | ---: |
| seeded | 0 | — | 0 |
| deleted | 8192 | — | 8192 |
| replaced | 4096 | 0 | 4096 |
| combined | 12288 | 0 | 12288 |
| replaced-deleted | 4096 | 12288 | 16384 |
| same-step | — | — | 0 |

All other filesystem layers must report zero. History-only entries have no
DiffID and no measurement. Values are logical file bytes, not compressed size
or guaranteed recoverable disk space. The Dockerfile independently defines these
sizes: old app 4096 bytes, cache 8192 bytes, new app 12288 bytes. In `same-step`,
the temporary file is removed before the layer is committed.

The fixture base copies only the pinned BusyBox executable into a scratch image
and installs its commands as **symlinks**, not hardlinks. This is a controlled
no-hardlink fixture: the original BusyBox image contains hardlinked commands,
which the initial collector deliberately rejects as unsupported. That rejection
must produce an absent measurement, not a zero result.

For manual inspection:

```sh
docker image inspect localhost/hidden-bytes/combined:qa \
  --format '{{json .RootFS.Layers}}'
docker image save localhost/hidden-bytes/combined:qa | tar -xOf - manifest.json | jq '.[0].Config'
test/fakeintake/build/fakeintakectl --url http://127.0.0.1:18080 \
  filter container-images --name localhost/hidden-bytes/combined | jq .
```

For independent archive inspection, `docker image save -o /tmp/hidden-bytes-combined.tar
localhost/hidden-bytes/combined:qa` saves only this fixture. Inspect the archive's
manifest and layer tar headers to confirm sizes and whiteouts; don't infer hidden
bytes from the final merged filesystem alone.

## 3. Restart, concurrency, and negative checks

Clear fakeintake between scenarios **after** the changed Agent is running, so old
reports cannot satisfy the assertions:

```sh
test/fakeintake/build/fakeintakectl --url http://127.0.0.1:18080 flush
```

- **Warm Agent restart:** restart only `deployment/agent`, wait for rollout, flush,
  and rerun the script. The node-backed `/var/run/hidden-bytes-agent` cache survives.
  Verify cache-hit evidence in debug logs as well as correct received values.
- **Cold cluster restart:** stop/start only profile `hidden-bytes-validation`.
  For a genuinely cold scan, set `DD_RUN_PATH` on this test Deployment to a fresh
  directory such as `/tmp/hidden-bytes-cold-1`; do not delete the node's runtime
  storage. Re-establish port-forward, wait for the Agent, flush, and assert again.
- **SBOM coexistence:** set `DD_SBOM_ENABLED=true` and
  `DD_SBOM_CONTAINER_IMAGE_ENABLED=true` on `deployment/agent`. The manifest already
  sets direct overlayfs scanning and fakeintake-only SBOM endpoints. Use a fresh
  `DD_RUN_PATH`, then assert hidden bytes and inspect `fakeintakectl get sbom ids`.
  Confirm no lease/view collision in logs during concurrent scans. Local-only
  fixture tags may lack repository digests and be skipped by the existing SBOM
  sender; cluster images such as CoreDNS can demonstrate SBOM emission instead.
- **Disabled or inaccessible:** set `DD_CONTAINER_IMAGE_HIDDEN_BYTES_ENABLED=false`
  and check `assert_payloads.sh http://127.0.0.1:18080 absent`. Separately re-enable it,
  use a fresh `DD_RUN_PATH`, and set `HOST_ROOT=/missing-host-mount`. Normal image
  metadata must still arrive, with `hidden_bytes` absent rather than zero. Restore
  `HOST_ROOT=/host` afterward. A warm cache would invalidate this negative test.
- **Same tag, new image:** rebuild a fixture with different file sizes under the
  same tag, reload it and recreate its workload. Record the changed config ID and
  expected values manually; the stock assertion script intentionally only checks
  the original fixture sizes. Verify the new result is not reused from the old ID.

Example of a namespace-scoped restart:

```sh
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  -n hidden-bytes-validation rollout restart deployment/agent
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  -n hidden-bytes-validation rollout status deployment/agent --timeout=180s
```

Example of a cold scan alongside SBOM:

```sh
minikube -p hidden-bytes-validation kubectl -- --context=hidden-bytes-validation \
  -n hidden-bytes-validation set env deployment/agent \
  DD_RUN_PATH=/tmp/hidden-bytes-cold-1 DD_SBOM_ENABLED=true DD_SBOM_CONTAINER_IMAGE_ENABLED=true
```

## 4. Prove independence from layer tarballs

A cluster restart alone **does not prove** tarballs disappeared. In the dedicated
profile, record each fixture manifest's layer blob digests and use
`sudo ctr -n k8s.io content ls` to establish whether those exact blobs are absent.
Also verify image config/manifest metadata and unpacked snapshots still exist.
Then use a fresh Agent `DD_RUN_PATH`, flush fakeintake, and assert exact values.

If blobs remain present, record this case as **not exercised**. Do not remove broad
content-store directories or arbitrary blobs: doing so can break the fixture,
Agent, or cluster. Any deliberate blob-removal experiment must first identify
only fixture-owned layer digests and preserve the image metadata/snapshot chain.

## Evidence and cleanup

Record the Agent/payload commit IDs, platform/runtime versions, fixture config
IDs/DiffIDs, script output, and which restart/missing-blob/SBOM cases were actually
run. A successful log line alone is not proof of emission. UI and backend totals
are intentionally not part of this change.

Leave the profile running for reviewer validation if requested. To stop just this
environment later: `minikube stop -p hidden-bytes-validation`. To discard it after
review: `minikube delete -p hidden-bytes-validation` removes the isolated cluster,
its fixture images and cache; that removal is not recoverable. It does not remove
the source checkout or locally built Docker images.
