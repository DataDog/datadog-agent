# Local validation

## Hardlinks and local archive fallback, 2026-09-23

Same retained ARM64 minikube profile and namespace, `hidden-bytes-validation`.
Both native and overlayfs snapshotter plugins are healthy. Fakeintake forwarding
remains disabled; no production credentials or registry fallback were used.

Built from Agent commit `f13eb1896a0fd972666fcc94cf5b42e6f4ee7e87` plus the
uncommitted hardlink/archive changes. Payload dependency remains
`cd3c28f1da19b2ed9862b3ea0d23d8f3dc2ad6b6`. Final local Agent image:
`localhost/hidden-bytes/agent:hardlinks-v2-qa`, config ID
`sha256:a81a50bb7b85128506732d803aada5097a0478098ba6e47ed81de3bcb513924e`.
The binary SHA256 is
`8b5efb7ecfe6e690f8078b267d9ba73cf51f3feaabf63adffa0505942c586063`.

### Received values

The assertion script checks exact config IDs, ordered DiffIDs, field presence,
and every filesystem layer's value. All original six fixtures retained their
expected values. New fixtures:

| Fixture | Seed layer | Replacement layer | Total for QA only |
| --- | ---: | ---: | ---: |
| hardlink-seeded | 0 | — | 0 |
| hardlink-one-deleted | 0 | — | 0 |
| hardlink-both-deleted | 4096 | — | 4096 |
| hardlink-replaced | 0 | 0 | 0 |
| hardlink-old-deleted | 4096 | 0 | 4096 |
| hardlink-all-deleted | 4096 | 12288 | 16384 |
| native-only | 4096 | 12288 | 16384 |

Exported seed-layer tar headers confirmed one 4096-byte regular file and a
hardlink to it, not two independent copies.

With the normal host mount restored and a fresh node-backed cache, all 13
fixtures also passed on the final binary. Source logs confirmed overlayfs scans
for the 12 ordinary fixtures, BusyBox, and the QA Agent image; native-only used
archives. SBOM payloads arrived concurrently for cluster images.
The etcd SBOM contained a CycloneDX result. After scans settled, no
`datadog-image-view-*` snapshots or leases remained.

A warm Agent restart retained all 13 exact results, with cache-hit logs instead
of rescans. The node-backed cache is
`/var/run/hidden-bytes-agent/qa-20260923-final/container-image-hidden-bytes-v2.json`.
Disabling the feature omitted hidden-byte fields on all 13 fixtures, even with
that populated cache; normal image metadata continued arriving. Fakeintake was
flushed after each rollout so previous runs could not satisfy these assertions.

With a fresh cache and `HOST_ROOT=/missing-host-mount`, all 13 fixtures passed
through local layer archives on the final binary. Source/completion logs confirm
the route. Cold scans of the retained inventory took about four minutes; initial
two-minute assertion attempts timed out before queued fixtures were scanned.
Rerunning after the queue drained passed. BusyBox 1.37.0 and the QA Agent image
also emitted hidden-byte values through archives; their observed totals were
zero, a smoke test rather than an independent correctness oracle.

The native-only image was imported with `ctr images import --local --snapshotter
native`, without creating a Pod. Its config ID is
`sha256:254a2d910a22587e823ed26142363a5486038e7df28c738c96d7613098620f2e`.
The final chain
`sha256:14a0c5d974597614a4fadd4f0f0171d182bb02f315e9eab16fa748f164cdd25b`
exists in native and returns `not found` in overlayfs. It reported 4096 bytes on
the seed layer and 12288 on the replacement layer via archives even with the
normal host mount available.

The existing etcd image supplied a non-destructive negative case: its layer blob
`sha256:cbeee09c6b35bb253c7bb1f44f87f98554ab621e1bf5f70bd2e53363333a1cca`
was already absent (`ctr content get` returned `not found`). With the host path
inaccessible, the Agent logged both source failures and emitted normal image
metadata with hidden-byte fields absent, not zero. No blobs were deleted.
After restoring the host mount and using a fresh cache, the same etcd config ID
emitted hidden-byte fields on every filesystem layer through snapshot scanning,
demonstrating that this route works without that compressed blob.

### Boundaries

- This is not universal containerd coverage: absent archives plus inaccessible
  snapshots, unsupported hardlink layouts/metadata, or scan limits still omit
  the measurement.
- The current direct-overlayfs SBOM configuration could not scan the native-only
  image, although hidden-byte archive fallback succeeded. This change does not
  broaden SBOM snapshotter support.
- Four trusted-overlay-xattr unit cases require privileges unavailable in the
  development shell. The minikube Agent has QA-only `CAP_SYS_ADMIN`.
- Cluster stop/start and replacing a fixture under the same tag were not rerun
  in this session. No image contents were extracted by the new archive scanner.
- User validation is still pending. Nothing has been merged.

### Reviewer rerun

The retained Agent has `HOST_ROOT=/host`, the node-backed cache above, and both
hidden-byte collection and SBOM enabled.

From `/home/bits/dd/datadog-agent-hidden-bytes`, with the retained fakeintake
port-forward at `http://127.0.0.1:18080`:

```sh
bash test/integration/container_image_hidden_bytes/assert_payloads.sh
bash test/integration/container_image_hidden_bytes/assert_payloads.sh \
  http://127.0.0.1:18080 present native-only
```

If the port-forward has exited, use the command in [README](README.md), replacing
`minikube` with `/tmp/hidden-bytes-minikube/minikube-linux-arm64` in this workspace.
The README also has cold-scan and fallback instructions. A warm rerun verifies
received values but does not prove a fresh scan.

## Historical run, 2026-09-22

The following describes the earlier overlayfs-only implementation, before the
hardlink and archive changes above; its hardlink limitation is now superseded.

Environment: ARM64, minikube v1.39.0 (Docker driver), Kubernetes v1.35.1,
containerd v2.3.4 with the native overlayfs snapshotter. Dedicated profile and
namespace: `hidden-bytes-validation`. Fakeintake forwarding is disabled.

Agent base: `5061317e9bbb358352d4133d72d94f6c14630af8`, with this worktree's
uncommitted changes. Payload base: `9584637d1527d2e12d4678372e11a4082f9980af`,
with the optional `hidden_bytes` schema change now published as payload commit
`cd3c28f1da19b2ed9862b3ea0d23d8f3dc2ad6b6`. The Agent dependency is pinned to
that revision; the draft PRs still require review and user validation.

## Observed at fakeintake

The assertion script verified exact config IDs, ordered layer DiffIDs, field
presence, and each layer value, not just totals:

| Fixture | Hidden bytes, summed for QA |
| --- | ---: |
| seeded | 0 |
| deleted | 8192 |
| replaced | 4096 |
| combined | 12288 |
| replaced-deleted | 16384 |
| same-step | 0 |

All six passed with SBOM disabled and again on the corrected overlay-attribute
implementation with a fresh scan cache and SBOM enabled. The replace-then-delete
case attributed 4096 bytes to the seed layer and 12288 to the replacement layer.

SBOM payloads also arrived for cluster images, including CoreDNS, etcd and
kube-proxy. Local-only fixture tags have no repository digest, so the existing
SBOM sender skips their SBOM payloads; this is separate from hidden-byte emission.
No operation-owned snapshot views remained after the completed scans.

Both negative checks passed for all six images: disabling the feature, and
enabling it with a fresh cache and an inaccessible host mount. Normal image
payloads still arrived with `hidden_bytes` absent, not zero.

A warm Agent restart also passed all six assertions, with cache-hit log evidence.
The retained Deployment has the host mount restored, both hidden-byte collection
and SBOM enabled, and the node-backed cache selected.

In this workspace, the minikube executable is
`/tmp/hidden-bytes-minikube/minikube-linux-arm64`. A fakeintake port-forward is
running at `http://127.0.0.1:18080`. From this worktree, rerun:

```sh
bash test/integration/container_image_hidden_bytes/assert_payloads.sh
```

## Constraints and remaining reviewer QA

- Standard BusyBox and the Agent base contain hardlinks; the collector correctly
  omitted their hidden-byte values. Controlled fixtures avoid hardlinks.
- Four trusted-xattr unit cases need privileges unavailable in the development
  shell; they were skipped. The actual Agent ran with QA-only `CAP_SYS_ADMIN`.
- A cluster restart, absent compressed blobs, and changing file sizes under the
  same tag have not yet been exercised. Use the steps in [README](README.md).
- A restart alone is not evidence of missing layer tarballs. No runtime blobs or
  storage directories were deleted in this validation.
- The profile is retained for user validation. Nothing has been merged.
