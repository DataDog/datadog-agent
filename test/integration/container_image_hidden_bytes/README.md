# Container image hidden bytes: manual QA

This validates the **Agent payload**, not a backend aggregate or UI. It checks
exact values received at fakeintake's `/api/v2/contimage` endpoint, using overlayfs
snapshots first and local layer archives when snapshots cannot be scanned.
A missing field is not zero. The Agent does not download missing image layers.

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
for scenario in seeded deleted replaced combined replaced-deleted same-step \
  hardlink-seeded hardlink-one-deleted hardlink-both-deleted \
  hardlink-replaced hardlink-old-deleted hardlink-all-deleted; do
  docker build -f test/integration/container_image_hidden_bytes/Dockerfile.fixtures \
    --target "$scenario" -t "localhost/hidden-bytes/${scenario}:qa" .
done

minikube start -p hidden-bytes-validation --driver=docker \
  --container-runtime=containerd --kubernetes-version=v1.35.1 --memory=6144 --cpus=4 --keep-context
for image in agent fakeintake seeded deleted replaced combined replaced-deleted same-step \
  hardlink-seeded hardlink-one-deleted hardlink-both-deleted \
  hardlink-replaced hardlink-old-deleted hardlink-all-deleted; do
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

Snapshot scanning requires native overlayfs metadata and `CAP_SYS_ADMIN` in the
initial Linux user namespace. It supports complete hardlink groups within one
layer; incomplete groups, cross-layer identities, remapped user namespaces and
metacopy/redirect metadata are unsupported. Failure falls back to local layer
archives, including for other snapshotters or inaccessible host mounts. Archive
hardlinks must refer backward to an existing regular-file object in the same
layer; forward or cross-layer links are unsupported. Missing archives, unsupported
metadata, or exhausted scan limits can still leave an image unmeasured. Partial
counts are never published; this is not universal containerd coverage.

The snapshot path reads metadata, not regular-file contents. The archive path
must read and decompress file bodies, but discards them without extracting files.
Each image scan has a one-minute timeout; archive compressed input and decompressed
output are each capped at 4 GiB, with additional metadata, entry and path limits.

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
  -n hidden-bytes-validation wait --for=condition=Ready pod/fixtures pod/hardlink-fixtures --timeout=180s
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
On a cold archive-only run, large images ahead of a fixture in the single-worker
queue can exceed that wait. Let the initial scans finish and rerun assertions;
check scan-source/completion logs rather than treating a polling timeout as zero.

| Fixture | Seed layer hidden bytes | Later replacement layer hidden bytes | Image total for QA only |
| --- | ---: | ---: | ---: |
| seeded | 0 | — | 0 |
| deleted | 8192 | — | 8192 |
| replaced | 4096 | 0 | 4096 |
| combined | 12288 | 0 | 12288 |
| replaced-deleted | 4096 | 12288 | 16384 |
| same-step | — | — | 0 |
| hardlink-seeded | 0 | — | 0 |
| hardlink-one-deleted | 0 | — | 0 |
| hardlink-both-deleted | 4096 | — | 4096 |
| hardlink-replaced | 0 | 0 | 0 |
| hardlink-old-deleted | 4096 | 0 | 4096 |
| hardlink-all-deleted | 4096 | 12288 | 16384 |
| native-only (separate import below) | 4096 | 12288 | 16384 |

All other filesystem layers must report zero. History-only entries have no
DiffID and no measurement. Values are logical file bytes, not compressed size
or guaranteed recoverable disk space. The Dockerfile independently defines these
sizes: old app 4096 bytes, cache 8192 bytes, new app 12288 bytes. In `same-step`,
the temporary file is removed before the layer is committed.

The fixture base copies the pinned BusyBox executable and libraries into scratch,
installing commands as symlinks. The `hardlink-*` fixtures add a true hardlink pair
sharing one 4096-byte file. Deleting one name hides no data while another name
survives. Replacing the first name explicitly unlinks it before creating a new
12288-byte file; truncating the shared inode would test different behavior.
Only deleting the final old alias hides the original 4096 bytes. Deleting the new
file afterward hides another 12288 bytes. Check the exported tar headers to prove
the builder preserved the intended hardlinks, not two independent copies.

Real pinned BusyBox and Agent images should also be scanned as smoke tests. Record
success or the exact unsupported case; fixture results do not prove their coverage
or independently establish their hidden-byte totals.

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
- **Archive fallback:** re-enable collection, use a fresh `DD_RUN_PATH`, and set
  `HOST_ROOT=/missing-host-mount`. With local fixture blobs present, the normal
  `present` assertions must still pass. Confirm the successful archive source in
  debug logs; payload values alone cannot prove which scanner ran. Restore
  `HOST_ROOT=/host` afterward. A warm cache would invalidate this fallback test.
- **Disabled or both sources unavailable:** set
  `DD_CONTAINER_IMAGE_HIDDEN_BYTES_ENABLED=false` and check
  `assert_payloads.sh http://127.0.0.1:18080 absent`. Separately test both unreadable
  snapshots and missing blobs with a fresh cache: metadata must arrive without
  `hidden_bytes`. Use focused provider tests if safely isolating missing runtime
  blobs is impractical; do not delete shared runtime content to force this case.
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

## 4. Test a native-only image

Build a distinct fixture and import it into containerd's **native** snapshotter.
Do not use `minikube image load` or create a Kubernetes Pod for this fixture:
either can unpack it into the default overlayfs snapshotter. Its unique last
filesystem layer ensures the full chain is not already among the overlayfs tests.
The Agent collects images in the `k8s.io` namespace even without a running Pod.

```sh
docker build -f test/integration/container_image_hidden_bytes/Dockerfile.fixtures \
  --target native-only -t localhost/hidden-bytes/native-only:qa .
qa_native_archive=$(mktemp /tmp/hidden-bytes-native.XXXXXX.tar)
docker image save -o "$qa_native_archive" localhost/hidden-bytes/native-only:qa
minikube -p hidden-bytes-validation cp "$qa_native_archive" /tmp/hidden-bytes-native.tar
minikube -p hidden-bytes-validation ssh -- \
  'sudo ctr -n k8s.io images import --local --snapshotter native /tmp/hidden-bytes-native.tar'
```

Confirm the final chain exists only in native. Run this snippet in **bash**:

```bash
qa_chain=""
while read -r qa_diff; do
  if [[ -z "$qa_chain" ]]; then
    qa_chain=$qa_diff
  else
    qa_chain="sha256:$(printf '%s %s' "$qa_chain" "$qa_diff" | sha256sum | cut -d ' ' -f 1)"
  fi
done < <(docker image inspect localhost/hidden-bytes/native-only:qa | jq -r '.[0].RootFS.Layers[]')
minikube -p hidden-bytes-validation ssh -- "sudo ctr -n k8s.io snapshots --snapshotter native info '$qa_chain'"
minikube -p hidden-bytes-validation ssh -- "sudo ctr -n k8s.io snapshots --snapshotter overlayfs info '$qa_chain'"
```

The native lookup must succeed; the overlayfs lookup must specifically report
`not found` (a connection/permission error does not establish absence). Use a fresh
Agent cache, restore `HOST_ROOT=/host`, wait for rollout, flush fakeintake, and run:

```sh
bash test/integration/container_image_hidden_bytes/assert_payloads.sh \
  http://127.0.0.1:18080 present native-only
```

Require both exact per-layer payload values and an archive-source success log for
this image. This proves fallback for a real non-overlayfs image with local blobs,
not support for every native/lazy/remote image. The assertion script also accepts
any subset of scenario names after its URL and `present`/`absent` arguments.

## 5. Prove independence from layer tarballs

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
