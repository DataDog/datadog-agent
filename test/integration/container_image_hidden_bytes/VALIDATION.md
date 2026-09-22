# Local validation, 2026-09-22

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
