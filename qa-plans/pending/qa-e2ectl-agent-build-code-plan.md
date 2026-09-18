# Agent builds: precise implementation plan

> **Partially implemented blueprint.** The original assessment below was source-only;
> see [implementation status](../notes/qa-e2ectl-receivers-artifacts-implementation.md)
> for the now-implemented subset and tests actually run. This document turns the
> [build design](qa-e2ectl-agent-build-plan.md) into file-level changes, and
> corrects assumptions that did not survive source inspection.

**Inspected baseline:** `056c4bd5215` on `rework-qa-experience`.
**Path convention:** framework paths below are relative to `test/e2e-framework/`;
`tasks/`, `packages/`, and `test/new-e2e/` paths are repository-root-relative.

## 1. Recommendation

Extract **artifact preparation**, not a new build system. Keep the existing
Invoke/Bazel engines and existing install/provision boundaries:

```text
installer requests a typed artifact
  → provider builds it, downloads it, or validates an existing artifact
  → verified result manifest + immutable staged files / image identity
  → installer delivers and activates it
  → snapshot records what was actually installed
```

Start with the two existing integrations: core binary and Agent image. Add
local-package installation before automating package production. Then pipeline
acquisition, selected additional binaries, and conservative caching.

Keep `Installer`/`Updatable` and provider registration CLI-owned. Public framework
code exposes typed requests, results and reusable build adapters; it must not
import `cmd/internal`, `envstore`, or Pulumi. Do not require the pending
custom-environment contract refactor to land first.

## 2. What exists — and what the original plan needs to correct

| Current source / symbol | Observed behavior | Consequence for this plan |
|---|---|---|
| `cmd/e2ectl/internal/installer/binary.go`: `Install`, `Update`, `buildAgentBinary`, `pinArtifacts` | Install removes the container **before** building. Update builds first, then removes and re-pins. Both use fixed worktree output paths. | Unify preparation before replacement; an unsuccessful build must not stop the working Agent. |
| Same file: `Update(skipBuild)` | Skips compilation but still calls `pinArtifacts`, copying the current `bin/agent/agent` and `dev/lib`. | It is **not** reuse of the environment's pinned build. Make new reuse semantics explicit, tested, and documented. |
| Same file: `pinArtifacts`, `writeSnapshotOutputs` | Copies selected `.so` names from `dev/lib`; snapshot architecture is hardcoded ARM64. Records `AgentBinPath`, but no artifact identity. | Runtime inventory and target platform must come from a verified build result; preserve the existing binary-path client integration. |
| `tasks/agent.py`: `build` (~57–227); `tasks/rtloader.py`: `install_with_bazel` (~99–149) | Core build defaults to Bazel-installed rtloader, CPython and dependencies under `dev/embedded` on Unix. `dev/lib` is the legacy CMake layout. Build also generates configs/assets. | The current binary installer can pick up stale legacy libraries. Do not codify its file glob as the shared build contract. |
| `cmd/e2ectl/internal/installer/installer.go`: `Kubernetes.Install`, `Update`, `buildAgentImage` | Install consumes/delivers an existing `helm.image`; **only Update builds it**. A version-only update does not build. | Preserve this behavior for old configs in the extraction; explicit new build configuration may opt install into building. |
| `tasks/agent.py`: `hacky_dev_image_build` (~451–671) | Overlays core and optionally rebuilt components onto a released base. Uses legacy rtloader with the base image's Python, patches paths, installs Rust-check assets, and may include eBPF assets. | False subagent flags mean **inherited from the base**, not absent. Record rebuilt versus inherited components; do not label this a “core-only image”. |
| Same task | Unspecified base resolves the latest stable release. `--arch` affects Docker/eBPF selection, not every nested compiler invocation. | Resolve/pin base identity for provenance. Reject unverified cross-build combinations instead of promising that `--arch` is sufficient. |
| `cmd/e2ectl/internal/installer/installer.go`: `HostScript`; `testing/installers/host/installscript/installscript.go` | Released-version install only; `HostScript` does not implement `Updatable`. Standalone installer writes YAML/conf.d and restarts systemd. | Local-package consumption is missing **in this standalone path**, not everywhere in the framework. |
| `components/datadog/agentparams/params.go`: `WithLocalPackage`, `WithPipeline`; `components/datadog/agent/{package.go,host.go,host_linuxos.go}` | Pulumi installs already select local packages, upload them, and use the OS package manager. | Reuse/extract that policy. Importing the whole `agent` or `components/os` package into e2ectl would reintroduce Pulumi. |
| `tasks/package.py`: `download` (~193–279); `tasks/libs/package/url.py` | Pipeline/PR acquisition exists for deb/rpm/docker/oci. Native packages extract by default; images become archives, not daemon-loaded images. | Use `--no-extract` for native-package installation; distinguish image archive, loaded image, and registry reference. |
| `tasks/omnibus.py`: `build`; Linux package CI; `packages/agent/linux/BUILD.bazel` | Omnibus remains in CI; Linux Bazel package targets also exist but migration is incomplete. `tasks/package.py` is acquisition tooling, not a package producer. | Adapt existing producers; do not build a new packaging engine or assume Bazel package parity. |
| `tasks/msi.py`: `build` (~373 onward) | Native Windows packaging has its own toolchain and task. | MSI is not “the same omnibus command with a platform flag”. Defer native Windows integration behind an explicit capability check. |
| `tasks/agent.py`: `build_remote_agent` (~757–762) | Builds `./internal/remote-agent`, the remote-agent **example client**, locally. | Remove it as a remote-build reuse candidate. Actual build offloading is separate future work. |
| `cmd/e2ectl/commands.go`, `envstore.Meta`, installer `Artifact` methods | `Artifact(cfg)` returns requested version/image, not verified build output. Metadata has no receipt. | Do not treat those fields as proof that a particular working-tree change is running. |

At the inspected baseline, no shared build provider registry, build-result manifest,
content-addressed cache, standalone local-package installer, or automatic
pipeline-build integration existed in e2ectl. The implementation status linked above
supersedes this historical inventory for current capabilities.

## 3. Separate the dimensions

Do not put `pipeline`, `remote`, `fips`, and `windows` into one `Kind` enum:

| Dimension | Proposed meaning |
|---|---|
| Artifact format | `binary-bundle`, `image`, `package-set` |
| Acquisition source | `local-build`, `existing`, `pipeline` |
| Execution backend | Invoke on the current build host initially; a remote backend later |
| Target | OS + architecture; package family/distribution where needed |
| Product / options | Agent flavor, components rebuilt from source, race/development flags |

`binary` is a bundle with one executable plus its required runtime/assets;
`binaries` is the same format with more component roles, not a different protocol.
An image can contain released subagents alongside a locally rebuilt core. The
record must say which is which.

## 4. First implementation slice: typed results and task-owned output discovery

### 4.1 Add a versioned result manifest at the producer boundary

**New:** `tasks/libs/agentbuild/{__init__.py,manifest.py}` and
`tasks/unit_tests/agentbuild_manifest_tests.py`.

`manifest.py` is an Invoke-free serializer/validator. It receives explicit paths
from the task that produced them; it does not search for the newest file. Provide
`write_result(path, result)` using a temporary file + atomic replacement.

**Modify:** `tasks/agent.py`:

- Add optional `result_manifest=None` to `build` and `hacky_dev_image_build`
  (`--result-manifest` on the CLI); keep existing invocations and logs unchanged.
- At the beginning invalidate any previous result at that requested path. On
  success write it only after compilation, asset generation and verification.
  A failed build must not leave a success manifest from an older invocation.
- `build` records the effective `agent_bin`, target, flags, runtime layout/root,
  Python home and asset directories returned/selected by the existing code.
  Distinguish no-Python, Bazel embedded, legacy CMake and Windows layouts.
- The image task records the resolved base identity, actual Docker image ID,
  repository digest if pushed, target platform, and rebuilt component list.
  It emits its own result **after** Docker build/push, not its nested core result.
- Preserve all build-tag computation in the current task helpers. Do not copy
  build-tag lists into Go, or force `--enable-bazel` with the existing binary
  installer's `--build-exclude=systemd` (that combination is rejected today).

**Manifest v1:** common header plus a format-specific payload. Define matching
Go structs and Python validation; exactly one payload is allowed.

- Header: schema version, producer ID/version, actual OS/arch/flavor,
  effective non-secret options, source commit, source-content digest when known.
- Binary payload: executable roles and paths; runtime layout/root and required
  Python ABI; config/check/eBPF assets; files with relative path, mode, SHA-256,
  and safe symlink information when packaged into a bundle.
- Image payload: source/base reference and immutable identity, built image ID,
  optional registry digest, archive path/type if acquired as a file, rebuilt and
  inherited component provenance. Do not invent a registry digest for an image
  that exists only in the local Docker daemon.
- Package payload: exact package files, format, version, architecture, flavor,
  distribution constraints, checksums and any required install ordering.

A single overloaded `Artifact.Ref string` is insufficient for these results.
Never include Agent API keys, signing keys or the process environment in a manifest.

### 4.2 Add a reusable, Pulumi-free build adapter package

**New:** `testing/installers/agentbuild/`:

| File | Proposed responsibility |
|---|---|
| `types.go` | `Target`, `Provenance`, `File`, `BinaryBundle`, `ImageArtifact`, `PackageSet`, typed request structs |
| `manifest.go` | Strict versioned decoding, checksum/path/platform validation; reject partial or mismatched output |
| `invoke.go` | Concrete `Invoke` adapter, argv-based subprocess execution with explicit cwd/context/log writers |
| `binary.go` | `Invoke.BuildBinary(ctx, BinaryRequest) (BinaryBundle, error)` |
| `image.go` | `Invoke.BuildImage(ctx, ImageRequest) (ImageArtifact, error)` |
| `bundle.go` | Stage verified files under a caller-owned output directory; preserve relative layout/modes/safe symlinks |
| `*_test.go`, `BUILD.bazel` | Fake-command and temporary-filesystem tests; no Docker/cloud/build toolchain needed |

API outline (new types, not existing APIs):

```go
type Invocation struct {
    Program string
    Args    []string
    Dir     string
    // Explicit, bounded environment overrides; never persisted wholesale.
}
type RunFunc func(context.Context, Invocation) error

type Invoke struct {
    Run RunFunc // production executor or a recording fake in tests
}

// Requests are different types; package-only knobs cannot leak into images.
func (b *Invoke) BuildBinary(context.Context, BinaryRequest) (BinaryBundle, error)
func (b *Invoke) BuildImage(context.Context, ImageRequest) (ImageArtifact, error)
```

Each request includes an absolute repository root, target and output directory;
image requests additionally carry base/target references and selected rebuilt
components. Public code knows neither `config.File` nor `envstore.Entry`.
No global registration side effects or framework dependency on CLI schemas.

### 4.3 Runtime relocation is a blocking acceptance gate, not a file-copy detail

The existing `pinArtifacts` approach is not sufficient for the default Bazel
runtime. `tasks/libs/common/utils.py:get_build_flags` bakes in a Python home,
and `tasks/rtloader.py:install_with_bazel` patches runtime RPATHs. `LD_LIBRARY_PATH`
alone does not prove the Python runtime is independent of the checkout.

**Required producer changes:** add opt-in bundle export parameters to
`tasks/agent.py:build` (`bundle_dir`, `runtime_prefix`) and implement staging in
`tasks/libs/agentbuild/bundle.py`. Keep command execution in the task wrapper.

1. Copy the actual runtime closure and assets reported by the producer, not a
   `dev/lib` glob. Stage copies; never patch libraries used by another build.
2. For a container-targeted bundle, use a fixed destination such as
   `/opt/e2ectl/runtime`. Distinguish link-time lookup paths from runtime Python
   home in `build`/`get_build_flags` parameter flow; the current Bazel branch
   unconditionally sets both to `dev/embedded` and needs this separation.
3. Reuse the existing Bazel `replace_prefix`/patchelf tooling for staged runtime
   load paths. It changes RPATH; it does **not** rewrite the Go-linked Python-home
   string. Both need explicit handling and tests.
4. Record image ABI constraints and verify the bundle inside the selected runtime
   image. Start with native Linux; reject unsupported OS/arch/runtime combinations.
   Do not assume `runtime-image` overrides are ABI-compatible.
5. Gate on a Python check as well as a Go check, with the original checkout/runtime
   inaccessible. A passing heartbeat alone does not exercise CPython or relocation.

Retain the image task's legacy-runtime path initially. It already matches Python
to its base image; migrating that task's implementation is a separate change.

## 5. Wire the existing installers without moving build logic into commands

### 5.1 Core binary installer

**Modify:** `cmd/e2ectl/internal/installer/binary.go`, `binary_test.go`.

- Replace `buildAgentBinary`, `buildAndPinBinary`, `pinArtifacts`, and `devLibSubpath`
  assumptions with the typed adapter result. Keep Agent config, conf.d seeding,
  container naming, fakeintake wiring, and readiness owned by the installer.
- Both install and update follow: resolve/validate target → prepare artifact in a
  fresh directory → verify runtime → stop old container → start using new immutable
  paths → readiness → publish result. Remove duplicate container deletion from
  `runAgentContainer` once the activation step owns it.
- Change `agentRunArgs` to consume bundle paths/runtime metadata. Keep
  `HostAgentOutput.AgentBinPath` as the in-container executable path; do not put
  local source paths into the client-facing field.
- Populate snapshot architecture from the verified target, not `ARM64Arch`.
  Inspect the selected runtime image for OS/distribution information instead of
  retaining unconditional Ubuntu 24.04 claims under arbitrary image overrides.
- New pins: `<env>/artifacts/<artifact-id>/...`, not mutable `agent-binary` and
  `dev-lib` files. Build to a unique temporary generation, then atomically publish
  it. A previous container may keep its generation mounted while compilation runs.
- Retain legacy pins for existing environments until an explicit successful rebuild.
  Do not invent provenance for them or silently reuse possibly stale `dev/lib`.

### 5.2 Helm installer and image delivery

**Modify:** `cmd/e2ectl/internal/installer/installer.go` and add
`kubernetes_test.go`; adjust `testing/installers/kubernetes/helm/{helm.go,helm_test.go}`.

- Replace `buildAgentImage` with `Invoke.BuildImage`; keep `DeliverImage`/kind load
  environment-owned. Verify platform before loading into the cluster.
- Consume a verified image identity. Give kind-delivered builds an immutable,
  semver-shaped unique tag derived from artifact identity; a reused mutable tag
  alone may not roll the DaemonSet. Registry consumers can use a digest.
- Deep-merge image overrides into `values["agents"]["image"]`. Current image-mode
  code replaces the entire `agents` map and loses user `customAgentConfig` and
  other sibling values. Add a regression test.
- Extend the shared Helm params/output construction to distinguish node-Agent,
  cluster-checks runner and Cluster Agent image/version roles. A core Agent image
  is not automatically a rebuilt Cluster Agent. Pin the released Cluster Agent
  when not rebuilt; do not silently set it to mutable `latest` for provenance.
- Preserve old-config behavior initially: version-only install/update never build;
  legacy `helm.image` install consumes the image, update rebuilds unless skipped.
  New explicit build configuration applies consistently to install and update.

### 5.3 CLI contracts, command cancellation, and installed records

**Modify:** `cmd/e2ectl/internal/installer/installer.go`, implementations and call
sites in `commands.go`, `main.go`, `internal/drivers/{local,kind,ec2host}`, plus
`lifecycle_test.go` and installer tests.

- Pass `context.Context` to `Install`/`Update` and image-delivery hooks. Create the
  command context at the CLI boundary (signal cancellation); adapters terminate
  the build process group on cancellation so nested compiler/Docker processes do
  not continue writing outputs. Use OS-specific process handling where supported.
- Return `InstallResult` from installation/update, carrying the verified installed
  artifact summary. Keep `Artifact(cfg)` only as a legacy requested-summary helper
  during migration; commands must no longer use it as installed-build identity.
- Generalize `--skip-build` help beyond images. Initially default to invoking the
  build as today, letting Go/Bazel use their own caches. Add automatic reuse only
  after §8 is implemented.
- For newly recorded artifacts, `--skip-build` means reuse the last successful
  verified artifact. Do not copy a newer arbitrary worktree binary, download a
  newer pipeline, or rebuild as a fallback. Missing/incompatible artifact → error
  before stopping anything. Document the changed legacy behavior explicitly.

**Modify:** `internal/envstore/envstore.go` and the snapshot publish helpers.

- Put the full non-secret receipt in snapshot metadata (e.g. `_agent_artifact`),
  not in `HostOutput`, and keep `meta.json` a compact display index.
- Extend `testing/provisioner/snapshot_bindings.go` with an atomic batch helper,
  `UpdateSnapshotResources(path, updates RawResources, metadata map[string]any)`.
  Make the existing single-resource helper delegate to it. Preserve unrelated
  resources, metadata and bindings; update bindings for every changed resource.
  Use it from `writeAgentToSnapshot`/`writeSnapshotOutputs` so `remoteHost`, the
  Agent output and its receipt are published together, not in separate writes.
- `InstallResult` supplies the actual version/image/artifact ID for `meta.json`.
  `list` stays a local-files operation; display an artifact ID and a legacy/unknown
  marker where no receipt exists. No Docker inspection or build during `list`.
- Track install outcome separately from infrastructure readiness. A build failure
  keeps the previous installation unchanged. If replacement has begun and readiness
  fails, report that failure, retain both generations/logs, and do not describe the
  previous artifact as currently healthy. Do not promise automatic rollback for a
  partially installed OS package.

## 6. Typed configuration and explicit extensions

### 6.1 Do not introduce the original plan's top-level `build:` bag

The current parser accepts only `schema`, `environment`, `agent`, `workloads`;
its schema engine rejects `any`, raw `yaml.Node` fields and custom codecs. The
original example `helm: {image: 7.99.0-dev}` also fails the existing full-image-ref
validator. Treat it as design prose, not usable configuration.

**Recommendation:** an optional `agent.build` acquisition selector beside
`agent.install` and its typed installer section. One installation owns this
selection; future multi-Agent scenarios keep their own slot-specific build
configuration rather than relying on a global environment build.

**Modify:** `cmd/e2ectl/internal/config/config.go`:

- Add `Agent.Build` as a selector + selected section node/bytes, using the same
  two-stage parsing pattern as `Agent.Install` (not a schema-reflected union).
- `parseAgent` accepts the reserved `build` key; `parseBuild` enforces one provider
  selector and its matching section, independent of YAML key order.
- Keep `config.File.Path`/source bytes and line/column diagnostics. Resolve paths
  against a documented repository-root invocation, not the stored config's new
  `$E2ECTL_HOME` location. Persist the resolved root for subsequent commands;
  do not infer it from the config's containing test directory.
- `Example` composes the selected provider's generated section when requested.
  Existing `init` output and old configs need no build block.

**New:** `cmd/internal/envconfig/agentbuild/{binary,image,existing,pipeline,package}/`
(each owns its data-only config/schema, added only when its provider ships), and
`cmd/e2ectl/internal/buildprovider/{provider.go,registry.go,provider_test.go}`.

- A registration owns ID, description, schema, pure validation, example, and a
  typed preparation factory. Follow `driver.Define`'s typed-closure pattern.
- Use separate typed registries for binary, image and package results. For
  example `Registry[agentbuild.ImageArtifact]` accepts
  `Define[P, agentbuild.ImageArtifact](...)`; it cannot accidentally return an MSI.
- The installer chooses its compatible registry. Duplicate IDs and mismatched
  output types fail registration/validation, not after infrastructure creation.
- Registration happens explicitly in the CLI composition layer. Framework users
  call the public concrete adapters directly, without importing CLI registries.
- New backends supply another adapter/registration; adding one does not add a
  switch to `cmdInstall`/`cmdUpdate`. Do not expose arbitrary shell command strings
  or arbitrary environment variables as a generic builder configuration.

**Proposed syntax — not supported yet:**

```yaml
agent:
  install: helm
  helm:
    image: local/agent:7.99.0-e2ectl
  build:
    provider: invoke-image
    invoke-image:
      base-image: registry.datadoghq.com/agent:7.83.0
      rebuild-components:
        - agent
        - trace-agent
      race: false
```

The provider resolves the base tag to an identity before recording inputs. The
`image` field remains the requested output name; the installed receipt records
the actual delivered identity. Target OS/arch comes from the attached environment
or an explicit compatible target, never silently from the developer's machine.
Provider-specific options such as `rebuild-components` map onto the task's existing
flags. Support only combinations whose toolchains are verified; no blanket `full`
or `windows` variant promise.

## 7. Native packages and pipeline artifacts — reuse the actual facilities

### 7.1 Make an existing package installable first

**Extract:** the pure selection policy from
`components/datadog/agent/package.go:GetPackagePath` into a public Pulumi-free
`testing/installers/host/packagefile/` package. It must use `components/os/types`,
not Pulumi-bearing `components/os`. Keep the old exported function as a wrapper
and preserve its current directory-selection tests. Do not import agentparams
only to get a flavor string. Direct-file inputs need real package metadata
validation; the current extension-only shortcut is not a target check.

**New:** `testing/installers/host/localpackage/{install.go,install_test.go}` and
`cmd/e2ectl/internal/installer/package.go` with a typed
`cmd/internal/envconfig/package/` section.

- Expose `localpackage.Install(ctx, env, Params)` taking a validated exact package
  file, not a release version. Register `agent.install: package` alongside `script`
  in `internal/drivers/ec2host/ec2host.go`; keep script semantics unchanged.
- Follow the existing Pulumi flow: copy to a private remote staging location,
  verify checksum, install via the correct package manager, write datadog.yaml
  and conf.d, restart, initialize `env.Agent`. Extract shared host configuration
  application from `installscript/installscript.go` into
  `testing/installers/host/configure/` rather than duplicating it.
- Existing package-manager implementations under `components/os` include Pulumi
  command types. Extract pure command/selection policy before sharing it; do not
  link those implementations into e2ectl. Start with Ubuntu/Debian deb; add rpm
  manager differences in a separately tested step. Unsupported OS fails early.
- Add error-returning `Host.CopyFileE` in `testing/utils/e2e/client/host.go` and
  make existing `CopyFile` wrap it: standalone installation must return upload
  failures rather than call a test assertion/`FailNow`. Initial package path is
  SSH-host-only; the existing SFTP implementation is not Docker copy support.
- Local unsigned-package permission is explicit and bounded to the requested
  package, not a permanent disabling of repository verification. No credentials
  in build arguments/receipts; host Agent settings remain YAML/conf.d.

### 7.2 Acquisition from an existing CI pipeline

**Modify:** `tasks/package.py:download` to optionally emit the same result manifest;
share serialization from §4. Keep existing flags/defaults for other callers.
Extend `tasks/unit_tests/{package_tests,package_lib_tests}.py`.

**New:** `testing/installers/agentbuild/pipeline.go` + CLI provider adapters.

- Invoke `dda inv package.download --pipeline=<id> --type=deb --arch=<target>
  --no-extract --path=<isolated-output>` plus the proposed result flag.
- Resolve a PR selector once to pipeline + commit. Persist concrete identity;
  do not cache “latest PR build” as immutable input.
- Reuse `tasks/libs/package/url.py`. These are published testing-repository/registry
  artifacts; completion of a binary-build job does not mean deploy/publish has
  finished. Report “artifact not published” separately from authentication failure.
- For Docker/OCI output, import the archive into the correct Docker daemon before
  kind loading, or resolve a registry reference. Validate archive/daemon/target
  identity; the current download task does not perform `docker load`.
- Framework image helpers already use pipeline **and commit**, flavor, registry
  conventions and existence checks. `BuildDockerImagePath` alone does not resolve
  a pipeline ID. Keep QA and release/nightly naming schemes distinct.
- No arbitrary GitLab-job/S3 artifact downloads in the first provider.

### 7.3 Produce complete packages and additional binaries later

- Add a local-package builder adapter over `tasks/omnibus.py:build` using an
  explicit build profile and isolated artifact destination. Add optional result
  output to the actual package-producing stage; collect exact outputs from that
  invocation, not stale `pkg/` contents. CI build and packaging/publish stages are
  distinct. Preserve existing Omnibus build caches and dependency stages.
- Keep a future Bazel-package adapter separate. Targets
  `//packages/agent/linux:debian` and `:rpm` exist; current migration TODOs prevent
  declaring them a universal replacement for release packaging.
- Windows MSI adapter wraps `tasks/msi.py:build` on a supported Windows executor;
  do not route it through a Linux Omnibus/cross-compile shortcut. DMG support is a
  separate adapter and installer capability.
- Extend binary manifests for selected component tasks
  (`trace-agent.build`, `process-agent.build`, `security-agent.build`,
  `system-probe.build`, `cluster-agent.build`). Include runtime/eBPF/check assets,
  not just executables. Give Cluster Agent its own output/image role.
- **Building is not starting:** the local binary installer currently replaces the
  image entrypoint and runs only the core Agent. A multiple-binary bundle also
  requires an explicit process-supervision/install mode and lifecycle tests;
  merely building trace-agent does not make the existing status suite pass.

## 8. Cache and concurrency after correctness

**New:** `testing/installers/agentbuild/{fingerprint.go,cache.go,lock.go}` plus tests.

- Keep engine-level incremental builds first. Automatic whole-artifact cache reuse
  is opt-in until the input closure is sound. A Git SHA plus a dirty boolean is
  never a cache key: two different dirty edits collide.
- Inputs include tracked content, relevant untracked files, effective options,
  manifests/modules/build-tag definitions, task/toolchain versions and immutable
  base/runtime identities. Include local replace dependencies outside the repo or
  disable automatic reuse. Define exclusion rules so generated output/cache dirs
  do not change the key, without ignoring source inputs such as `invoke.yaml`.
- Snapshot inputs before and after a build; if they changed during compilation,
  do not publish a reusable success under the earlier key. Ask for a stable retry.
- Local Invoke tasks share `bin/`, `dev/`, generated files and sometimes process
  environment: serialize them per resolved worktree through staging completion,
  not just per cache key. Two variants in one checkout can otherwise race.
- Serialize activation per environment; distinct environments may use one immutable
  verified artifact. Environment-owned copies/reflinks remain valid if a shared
  cache is cleaned. No hardlinks to build outputs that may be overwritten.
- Verify checksums/image availability on reuse. A missing artifact with explicit
  `--skip-build` is an error; ordinary auto mode may prepare it again.

## 9. Delivery sequence and concrete gates

| PR | Scope | Required gate |
|---|---|---|
| A | Task result manifest + public binary/image adapters | Existing task invocations unchanged; manifest schema/paths/options/identity tests; no stale result after failures |
| B | Correct runtime bundle, installer preparation-before-stop, immutable pins, installed receipts, context propagation | Build/runtime mismatch fails before replacement; Go **and Python** checks work without access to the build checkout; version-only Helm path never builds |
| C | Explicit typed providers/config and unified opt-in install/update semantics | Unknown/wrong-kind providers rejected offline; generated examples decode; old configs keep documented behavior |
| D | Existing deb package installer and shared selection/config policy | Correct package uploaded/verified/installed on Ubuntu; failures returned normally; no Pulumi in CLI dependencies |
| E | Pipeline package/image acquisition, then local package production and component bundles | No-extract package test; image archive loading test; target/flavor validation; package publication and authentication errors distinguished |
| F | Content-keyed reuse and locking | Two dirty edits differ; changed base differs; concurrent builds cannot corrupt another environment; incomplete cache entries never reused |

Additional regression tests to write:

- `internal/installer/binary_test.go`: install/update/reuse call ordering, old
  container survives compile failure, multi-generation mounts, truthful architecture,
  `AgentBinPath` and legacy receipt migration.
- `internal/installer/kubernetes_test.go`: no local build on released version,
  image-only install compatibility, rebuilt components/base propagation, image load
  ordering, image values preserve sibling chart values, new identity rolls pods.
- `cmd/e2ectl/lifecycle_test.go`: generic dispatch with fake builders; no tasks in
  command code; failed readiness is not recorded as successful; workload deployment
  failure is distinguishable from an already successful Agent installation.
- `internal/config/config_test.go` and new schema tests: selector-order independence,
  type errors, source path resolution, invalid example image refs, unsupported
  platform/component combinations, no credential resolution in `init`/validation.
- `testing/provisioner/snapshot_bindings_test.go`: receipt + output atomic publication,
  existing bindings preserved, legacy snapshots still attach.
- Python producer tests: default `dev/embedded` versus legacy `dev/lib`, no-Python
  bundle, symlink traversal rejection, exact package output selection, no stale
  result after compile/push/download failure, no credentials in result files.

Validation commands **for the implementation**, not claimed as run here:

```sh
dda inv invoke-unit-tests.run
bazel test //test/e2e-framework/testing/installers/... \
  //test/e2e-framework/testing/provisioner/... \
  //test/e2e-framework/cmd/internal/... \
  //test/e2e-framework/cmd/e2ectl/... --test_output=errors
bazel query 'filter("pulumi", deps(//test/e2e-framework/cmd/e2ectl:e2ectl))'
# The query must remain empty.
```

Add live gates in `test/new-e2e/tests/agent-build/`: native binary-bundle health,
Python-check runtime loading, minimal kind image heartbeat, and package install on
an explicitly provisioned Ubuntu host. Keep `e2ectl-local.yml`, `e2ectl-kind.yml`
and the small Python check fixture beside those tests. Reuse existing framework
clients and attach helpers; do not rewrite the containers or subcommands suites.
Run through `dda inv new-e2e-tests.run` with bounded `--targets` and `--run`, or
the supported Bazel test path. Do not make a new wrapper around raw
`go test` to bypass repository build/test policy. The entire containers suite is
not the initial gate: its known architecture/config/version gaps would obscure
build-layer regressions.

## 10. Explicitly out of scope

No remote build service, cloud provisioning changes, automatic plugin discovery,
arbitrary command hooks, full multi-Agent schema, or standalone `e2ectl build`
command in the initial slice. Cross-build and Windows entries are capability-gated
follow-ups, not advertised support. Agent runtime config remains an installer
concern, and building a package does not prove that an installer or service manager
can consume it.

This plan does not change tests or application code. The existing user edit to
`test/new-e2e/tests/agent-metric-emission/metric_emission_test.go` is outside its scope.
