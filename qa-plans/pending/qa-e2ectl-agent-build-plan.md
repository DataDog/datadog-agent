# e2ectl: an agent-build layer — one contract, many build kinds

> **Category C — pending plan.** How e2ectl and the e2e framework should
> integrate agent builds: single binary, dev docker image, full omnibus
> package, prebuilt pipeline artifacts — as one extensible contract.
> Ideas and design; no code. See the
> [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** plan only, grounded in the current installers and the repo's
invoke tasks.

## 1. The problem

Every installer needs a *different artifact form*, and each currently wires
its own build by hand:

| Installer | Artifact it needs | How it builds today |
|---|---|---|
| `binary` (local container) | the core agent **binary** + rtloader libs | `dda inv agent.build` → pins `bin/agent/agent` + `dev/lib/*.so*` into the env dir |
| `helm` (kind) | a dev **docker image** | `dda inv agent.hacky-dev-image-build --target-image=<ref>` → `kind load` |
| `script` (ec2-host) | a released **package** | nothing — released versions only; no way to test a local build on a VM |
| (nothing) | full binaries set / omnibus deb/rpm/msi | not reachable from e2ectl at all |

The build knowledge (which dda task, which flags, which side artifacts) is
scattered across installers, and each new artifact form means a new bespoke
integration. Meanwhile the repo already has every build kind as invoke tasks:
`agent.build` (single binary), `agent.hacky-dev-image-build` (image, with
`--process-agent/--trace-agent/--system-probe/--security-agent/…` toggles),
`omnibus.build` (packages), plus pipeline-image helpers in the framework
(`utils.BuildDockerImagePath`).

## 2. Principles (extend what exists)

1. **The installer owns its artifact** — established for `Updatable.Update(skipBuild)`:
   each installer prepares what it consumes. The build layer generalizes
   "prepare" without moving the knowledge into the CLI.
2. **The CLI stays build-ignorant.** `cmdUpdate` dispatches; it never knows
   a dda task name.
3. **Explicit registration**, like drivers and installers — no `init()`
   magic, no runtime plugins.
4. **Reuse the sanctioned build engine.** The invoke tasks (via `dda inv`)
   are the single source of truth for how an agent gets built; the contract
   wraps them, it does not replace them.
5. **Builds are optional inputs.** Every installer keeps working with
   released artifacts; local building is an opt-in upgrade path.

## 3. Build kinds taxonomy

| Kind | Produced by | Consumed by | Speed | Notes |
|---|---|---|---|---|
| `binary` | `agent.build` | binary installer (container host), future local-host installs | fastest (single Go binary) | needs rtloader libs pinned alongside; single core agent only — no trace/process agents |
| `binaries` | `agent.build` + friends | host installs wanting subagents (the status-suite boundary is exactly this) | fast | the full set: agent, trace-agent, process-agent, security-agent, system-probe |
| `image` | `agent.hacky-dev-image-build` | helm installer (kind) | minutes | flags already parameterize which subagents ship in the image — surface them |
| `package` | `omnibus.build` | script installer (VMs), full host parity | slow (minutes–tens of minutes) | deb/rpm (and msi on Windows); makes "test my branch on a real VM" possible |
| `pipeline` | CI (prebuilt) | any installer, instead of building | none (download) | pull the dev image/binaries from the agent QA registry by pipeline ID — reproducible, no local toolchain; the framework already models this for images |
| `remote` | `agent.build_remote_agent` | anything, from a weak laptop | network-bound | offload heavy builds (omnibus) to a beefy machine |

## 4. Proposed shape

### 4.1 The contract

A small shared package in the framework (e.g. `testing/installers/agentbuild`)
so scenarios and e2ectl consume the same thing:

```go
// Kind is the artifact form: binary, binaries, image, package, pipeline.
type Kind string

// Artifact is what a build produced: a form, a reference (path, image ref,
// package file set) and the fingerprint of the inputs it was built from.
type Artifact struct {
    Kind        Kind
    Ref         string            // binary path, image ref, package paths…
    Fingerprint string            // git SHA + dirty flag (+ task variant)
}

// Builder produces one artifact kind. Implementations wrap dda inv tasks
// today; a pipeline builder would download instead of building.
type Builder interface {
    Build(ctx BuildContext) (Artifact, error)
}

// BuildContext carries what a Builder needs: the variant flags (which
// subagents), the target platform, whether to push an image, and a cache
// directory.
type BuildContext struct {
    Variant  Variant   // core-only, full, fips, windows…
    Platform Platform  // host, amd64/arm64 cross-compile
    CacheDir string
    // …flags mirroring hacky-dev-image-build (trace_agent, process_agent…)
}

// Registration mirrors drivers/installers:
func Define(kind Kind, b Builder)
```

### 4.2 Who calls it

The installers, inside their existing prepare step — the contract slot
already exists (`Update(skipBuild)`; install takes an analogous path):

- **binary installer**: `agentbuild.Get("binary").Build(...)` → pin as today.
- **helm installer**: `Get("image").Build(...)` with the image ref from the
  agent section, then `kind load` (existing `DeliverImage` hook).
- **script installer (future)**: `Get("package").Build(...)` → upload to the
  VM and install the local package instead of the released one — this turns
  the VM path into a real build-target.

### 4.3 The config surface

Two candidate expressions, not mutually exclusive:

- **Derived (default)**: the artifact kind follows the installer — `binary:`
  installs a binary build, `helm.image:` builds an image. Zero new config for
  the common path.
- **Explicit (escape hatch)**: a `build:` block next to `agent:` for what the
  installer cannot infer — variant and platform:

  ```yaml
  agent:
    install: helm
    helm: {image: 7.99.0-dev}
  build:
    variant: full            # core-only (default) | full | fips
    platform: host           # or amd64/arm64/windows for cross-builds
    source: pipeline-1234    # default: local tree; pipeline = prebuilt
  ```

  `source: pipeline-<id>` selects the prebuilt path with no local build at
  all.

### 4.4 Caching and fingerprints

- `update --skip-build` already exists; make skip the *default when nothing
  changed*: fingerprint the working tree (git SHA + dirty marker + variant)
  and reuse the pinned artifact when it matches.
- Keep per-environment pins (current behavior — environments are
  independent) plus a shared cache keyed by fingerprint, so ten environments
  on one build do not rebuild ten times.
- `e2ectl list` gains an artifact column (which fingerprint each environment
  runs) — debugging "is my change actually deployed" becomes a one-glance
  answer.

### 4.5 A standalone `e2ectl build` (optional, later)

Build without deploying — useful to warm the cache or to fail fast on
compile errors before any environment churn:

```sh
e2ectl build --kind image --variant full
```

Not required for the contract; it falls out of it.

## 5. Ideas gathered (deliberately not all in v1)

- **Pipeline artifacts as first-class builds** — the fastest correct loop for
  QA: a CI pipeline builds once, every developer attaches. Needs artifact
  naming conventions + a `source:` selector.
- **Remote builds** — omnibus on a laptop is painful; `agent.build_remote_agent`
  or a plain SSH offload fits the e2ectl-worker pattern (a separate process,
  the CLI stays thin).
- **Cross-compile matrix** — build arm64 on amd64 and vice versa; matters for
  the ec2-host path (pick the VM's arch, not the laptop's).
- **Windows** — `binary` gets an exe flavor, `package` an msi; the windows-host
  environment type (exists in the framework) becomes reachable.
- **Subagent toggles surfaced** — `hacky-dev-image-build` already knows
  `--trace-agent/--process-agent/--system-probe/--security-agent`; the
  variant concept should map onto those instead of inventing new names.
- **Signed/reproducible builds** — out of scope for local iteration; noted so
  the Artifact struct leaves room (digest field).
- **Post-build hooks** — e.g. stripping, race detector builds (`--race`
  exists on the image task).

## 6. Suggested sequence

| Step | What | Why first |
|---|---|---|
| 1 | Extract the `agentbuild` contract (Kind/Artifact/Builder/Define) and re-home the two existing builds on it (binary, image) | proves the abstraction without new behavior |
| 2 | Fingerprint cache + skip-when-unchanged | the biggest daily-time win |
| 3 | `pipeline` builder (prebuilt artifacts) | fast + reproducible QA loops |
| 4 | `binaries` kind + the script-installer local-package path | unlocks "my branch on a real VM" |
| 5 | `package` (omnibus) + remote/cross-compile | completeness |

## 7. Open questions

- Does the `build:` block live next to `agent:` (environment-local) or is it
  global per developer (a shared cache config)? Leaning: config stays
  environment-local, cache stays global.
- Should `Variant` be a closed enum or a free string registered by builders?
  Closed-with-registry, matching the driver philosophy.
- Where do pipeline artifacts live — the existing QA registry only, or also
  S3 artifacts from arbitrary pipelines?
- Windows day-one or deferred? (Leaning deferred; the contract accommodates.)
