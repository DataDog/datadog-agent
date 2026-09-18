## Agent artifact sources

`agent.install` chooses the consumer; optional `agent.build.provider` chooses the
producer/acquirer independently. Provider sections are typed, reject unknown
fields, and are shared by install/update. Main commands and environment drivers
have no task names or artifact-format dispatch. All paths in **new** provider
sections must be absolute; they are not relative to the stored config copy.

| Installer | Providers | Current target |
|---|---|---|
| `binary` (local) | `invoke-binary`, `existing-binary` | Native Linux amd64/arm64; matching Ubuntu runtime image/Python ABI |
| `helm` (kind) | `invoke-image`, `existing-image` | Linux, homogeneous cluster architecture; local Docker image delivery |
| `package` (ec2-host) | `existing-package`, `omnibus-repackage` | Ubuntu SSH host, exact datadog-agent DEB; no RPM/MSI |
| `script` | none | Unchanged released install-script behavior |

For example, this replaces only the Agent on an existing kind environment (set
`repository` to your actual checkout; retain its existing environment settings):

```yaml
schema: 1
environment:
  base: kind
  fakeintake: true
  kind: {version: 1.33.0, nodes: 0}
agent:
  install: helm
  helm:
    version: 7.83.0
    values: |
      agents:
        customAgentConfig:
          log_level: debug
  receiver:
    type: fakeintake
    fakeintake: {remote-config: disabled}
  build:
    provider: invoke-image
    invoke-image:
      repository: /home/me/datadog-agent
      reference: localhost/datadog-agent:7.99.0-dev
      base-image: registry.datadoghq.com/agent:7.83.0
      rebuild-components: [trace-agent, process-agent]
      race: false
```

`invoke-image` wraps `dda inv agent.hacky-dev-image-build`. Core is always rebuilt;
other components are inherited unless listed. Also supported: `security-agent`,
`system-probe`, `trace-loader`, `privateactionrunner`. This task deliberately uses
legacy rtloader **against its image's Python**, unlike `invoke-binary`, which wraps
default `dda inv agent.build --build-exclude=systemd` and inventories `dev/embedded`.
Neither path uses a stale `dev/lib` glob. Native builds only; `--arch` alone does
not make every nested component task a cross-compiler.

### Build once, then consume the same artifact

Successful installs export non-secret receipts under
`<environment-dir>/artifacts/receipts/<receipt-sha256>.json`; the full installed
record is also in snapshot `_agent_artifact`. Optional `--result-manifest=PATH`
on `agent.build`, `agent.hacky-dev-image-build`, and
`omnibus.build-repackaged-agent` exposes the task boundary directly, without
changing default invocations. A failed producer invalidates its requested old
manifest. Receipts contain verified files/checksums, target, runtime inventory,
actual daemon image identity, and known source/base provenance—not an input cache
fingerprint. The source-content hash describes observed checkout content, not a
reproducible-build or signature certification.

To reuse the image above, replace **only** `agent.build` with:

```yaml
  build:
    provider: existing-image
    existing-image:
      reference: localhost/datadog-agent:7.99.0-dev
      manifest: /home/me/artifacts/image-result.json
```

Copy/use an actual exported image receipt for `manifest`. Its image ID must still
match the actual local image. Omit the manifest only for visibly legacy routing;
explicit receivers never silently fall back. A provider-generated local image
with the tested 7.83.0 base can attest the current source route contract. Unattested
arbitrary images/binaries, other image bases, and current package/repack providers cannot claim
that contract. A semver tag or DEB version alone is not capability evidence.

Images must already be available to `existing-image`; it does not pull, build or
refresh a missing artifact. Delivery uses an ID-specific
`7.99.0-e2ectl.<daemon-id>` tag, so rebuilding the same mutable source reference
changes the pod template. Node Agent/runner image overrides preserve sibling Helm
values. The separate Cluster Agent stays released 7.83.0, not a retagged core image.

For a local binary, use `agent.install: binary`, `agent.binary: {}` and either:

```yaml
  build:
    provider: invoke-binary
    invoke-binary: {repository: /home/me/datadog-agent, race: false}
```

or `provider: existing-binary` with
`existing-binary: {manifest: /home/me/artifacts/binary-result.json}`. The latter
requires an actual bundle receipt, not an arbitrary executable path.
Managed binary routing additionally requires the bounded core-source profile
emitted by `invoke-binary`. Existing unprofiled receipts need an explicit rebuild;
runtime probes alone never establish endpoint-coverage capability.

Binary generations contain the complete embedded runtime and generated assets,
plus an explicit immutable runtime-image dependency for `datadog_checks` and its
site-packages. This is **not a relocatable/self-contained binary distribution**:
the staged runtime mounts at its recorded link-time absolute prefix. Only the
staged copy is mounted, never the checkout's live runtime. Python ABI, platform,
image identity, file modes/checksums and safe symlinks are verified. Disposable
`--network=none` probes run the staged Agent, a Go check and a Python check importing
`datadog_checks.base`, `ssl` and `sqlite3` before replacing a working Agent. A custom
runtime must pass those gates; there is no pip-install or legacy-library fallback.

### Existing DEB and Omnibus repack

The package installer is available beside `script` on EC2 Ubuntu. For example:

```yaml
schema: 1
environment:
  base: ec2-host
  fakeintake: true
  ec2-host: {os: ubuntu-22.04, arch: amd64}
agent:
  install: package
  package:
    allow-unsigned: true
    config: |
      log_level: debug
  build:
    provider: existing-package
    existing-package:
      path: /home/me/artifacts/datadog-agent.deb
```

This intentionally omits `receiver`: package capability evidence is not yet
validated, so only the existing **legacy** receiver behavior is supported here.
That mode can resolve native runner credentials and has partial forwarding
coverage; do not treat it as egress isolation. Explicit package receiver requests
fail before building/uploading. No cloud package smoke is implied by this example.

For repackaging, keep the same installer and select `provider: omnibus-repackage`.
Its section requires `repository`, `build-image`, `base-package-url`, and
`base-package-sha256`. Supply the exact credential-free HTTPS URL and published
SHA256 from the selected repository's `Packages` metadata—not a fabricated digest
or a moving “latest” selector. The Docker build image must already be present and
contain `dda` plus the native Omnibus toolchain. The provider runs the existing
`dda inv omnibus.build-repackaged-agent` **inside that isolated container** with
those explicit base options and an invocation-owned output directory. Existing
callers omitting these options still select the latest nightly as before.

**Never run that task blindly on the host:** its `/opt/datadog-agent` overwrite
prompt remains intact. The adapter neither bypasses the prompt nor mounts the
host installed tree, Docker socket, or privileged devices. The repack flow has
only offline command-contract coverage here; a full safe build-container run and
Ubuntu installation remain required before claiming repack receiver compatibility.

Package metadata/target and local checksum are checked before upload; a unique
private SSH staging path is checked again remotely before apt runs. Unsigned-file
permission is explicit and does not change repository trust policy. Same-version
updates force apt reinstallation. Before success, installed dpkg
status/version/architecture and every declared executable role are compared against
the checksum-verified uploaded archive. Role extraction uses fixed whitelisted
paths and stdout into private staging, rejecting non-regular/duplicate members.
`_agent_artifact.packageVerification` records the scope and executable hashes:
this is **not** full conffile/runtime byte-for-byte verification. Executables
modified by postinst are unsupported and fail closed.

Package manager/upload errors return normally (no `testing.FailNow`). Existing
permanent and runtime masks are both rejected; systemctl inspection failures are
not interpreted as unmasked state. The installer creates exclusive runtime masks
with private inode anchors and rechecks device/inode/target evidence before
removing only its own masks, with systemd reloads. Foreign/admin replacements are
preserved and block restart. Owned masks/anchors remain for explicit repair on
failure. Services remain masked until shared private configuration is applied.
There is no promised rollback of a partly installed package.

### Reuse, compatibility and extension

- Existing image/package providers **never build**, including ordinary update.
  Legacy `helm.image` now means existing-image on both install and update; select
  `invoke-image` explicitly to retain a local rebuild loop. Version-only Helm and
  standalone script installs keep their released behavior. A Helm provider
  reference must not also be set in `agent.helm.image`.
- Legacy binary configs still build from the invocation's repository root on a
  normal install/update, but new builds use verified embedded-runtime generations.
  Old unprofiled pins are not sufficient for managed receiver-only apply.
  Explicitly rebuild with `invoke-binary` after any required state migration;
  no automatic adoption or re-attestation is performed.
- `update --skip-build` requires the last successfully installed receipt and
  verifies its environment-owned files/image. Missing or changed pins are errors,
  never a source repin/build fallback. Normal preparation finishes before activation.
  Binary receiver-only apply also reuses those pins and persistent runtime state;
  it never needs to build again. Helm/package receiver-only apply is still unsupported.
- Mutable Agent runtime/RC state is stored in an explicitly owned Docker named
  volume, separate from immutable artifact receipts. Its name derives from the
  environment directory and creation ID; an owner label and snapshot
  `_agent_runtime_volume` record are verified before reuse/removal. Startup sets
  the volume root to mode 0700 without changing host-directory permissions. Normal
  local stop removes the Agent first, then only this exact owned volume; no prune.
  Failed installs retain deterministically recoverable ownership. Install/update,
  receiver apply and local teardown share the per-environment operation lock.
- Intermediate receiver installs that already have meaningful bind-mounted
  `agent-run` state require explicit migration/recreation before install/update.
  The gate runs before building or stopping anything. Receiver-only apply also
  requires a valid core-source capability receipt; unprofiled `_agent_binary`
  snapshots cannot be adopted automatically. Empty scratch directories and
  original pre-receiver installs remain usable. Root-owned legacy state may need
  an explicitly authorized, narrowly scoped cleanup: `stop --force` does not fix
  filesystem permissions. There is no automatic live RC-state migration.
- Generations are retained independently per environment. There is no automatic
  artifact cache, dirty-boolean cache key, rollback, or generation garbage collector.
  Checkout builds and CLI Agent operations have exclusive locks; inspect a killed
  operation before manually removing a stale lock. Failed activation is recorded
  separately from infrastructure readiness in the snapshot.
- To add a source, implement a public adapter in `testing/installers/agentbuild`
  returning a verified result, add its data-only type under
  `cmd/internal/envconfig/agentbuild`, and explicitly `Define[P, Result]` it in the
  compatible `buildprovider` registry. Add schema, fake-executor, target, failure,
  checksum and no-build reuse tests. No main-command or driver format/task switch
  is needed. Framework callers use `agentbuild.Adapter` and
  `localpackage.Install(ctx, env, params)` directly, without CLI config/store/Pulumi.

## Validation and extension references

The [receiver guide](../../receivers/README.md) describes capability and routing
limits; the [CLI guide](../../../cmd/e2ectl/README.md) covers environment lifecycle.
An opt-in, network-disabled package regression runs real apt/dpkg only inside a
disposable Ubuntu container (no host mounts or host maintainer scripts):

```sh
bazel test //test/e2e-framework/testing/installers/host/localpackage:localpackage_test \
  --test_strategy=standalone --test_env=E2ECTL_PACKAGE_SMOKE_IMAGE=ubuntu:24.04 \
  --test_filter=TestSameVersionPackageReplacementInContainer --test_output=errors
```

The image must already exist locally. This covers same-version changed executable
bytes, status, ownership-safe masks and postinst mismatch rejection—not real SSH,
systemd or Agent startup. Full Omnibus/Ubuntu deployment still requires separate
authorization and a known isolated build image.
