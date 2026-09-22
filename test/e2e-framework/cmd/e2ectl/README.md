# e2ectl — QA environments for the Datadog Agent

Spin up a named environment (a local kind cluster, a local Agent container, or
an EC2 VM), install and update the Agent on it independently, see what the
Agent really sent through a fakeintake, and run your tests against the live
environment — no Pulumi for local operations.

## Install

From the repository root, using the Go toolchain required by `go.work`:

```sh
go install ./test/e2e-framework/cmd/e2ectl

# Optional: also install the matching worker for EC2 provisioning.
go install ./test/e2e-framework/cmd/e2ectl-worker
```

The binaries go into `GOBIN` (default: `$(go env GOPATH)/bin`); add that
directory to your `PATH`. Use a checkout rather than `go install …@latest`:
the framework depends on local modules through `replace` directives.

For kind environments, you also need Docker running, the `kind` and `kubectl`
CLIs, and API/application keys (`~/.test_infra_config.yaml` or `E2E_API_KEY`
and `E2E_APP_KEY`).

## First environment (~5 minutes)

```sh
e2ectl init --base kind --output my-kind.yaml    # annotated starter config — review it
e2ectl start --config my-kind.yaml --name dev    # kind cluster + fakeintake
e2ectl install --env dev                        # released Agent via Helm
e2ectl fakeintake metrics --env dev --name datadog.agent.running
```

That's the loop: the last command shows the Agent's heartbeat — you just
deployed a working Agent. To iterate on Agent code:

```sh
# ...edit Agent Go code...
e2ectl update --env dev                # rebuild from the checkout + redeploy
```

To iterate on your checkout instead of the released chart, set `source: true`
in `my-kind.yaml` before installing: e2ectl builds the hacky development
image from the checkout, loads it into the cluster and deploys it — the
`agent:` section is the whole installation surface (see below).

And to clean up: `e2ectl stop --env dev` (or `e2ectl list` to see what you have).

## Run tests on it

`e2ectl test` runs a new-e2e suite attached to a live environment — the same
test bodies as CI, only the provisioning differs:

```sh
# the agent-health suite, against a local Agent container:
e2ectl start  --config test/new-e2e/tests/agent-subcommands/e2ectl-local.yml --name my-health
e2ectl install --env my-health
e2ectl test --env my-health --suite ./test/new-e2e/tests/agent-subcommands/ \
  --run 'TestLinuxHealthSuiteOnLocal/TestDefaultInstallHealthy'

# the ORIGINAL containers suite, against a local kind cluster:
e2ectl start  --config test/new-e2e/tests/containers/e2ectl-kind.yml --name my-kind
e2ectl install --env my-kind
e2ectl test --env my-kind --suite ./test/new-e2e/tests/containers/ -run TestKindSuiteOnLocalKind
```

The config lives next to the test it serves. Attachable entry points are named
`<Test>On<Local|Host>` and skip themselves when no environment is attached,
so the same suites keep working in CI.

`e2ectl test` always passes `-count=1`: the attached environment is mutable
state (receiver switches, reinstalls, new payloads) that go test's cache
cannot see, so a cache hit would be a false PASS against a changed
environment. Pass `--cached` to opt back into caching, or `-- -count=N` to
override the count.

## Write your own test

An attach entry is three helper calls; the body is plain new-e2e:

```go
func TestMyCheckOnLocal(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)     // skips when E2ECTL_ENV is unset
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &mySuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Kubernetes](envName),
	))
}

func (s *mySuite) TestHeartbeat() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.agent.running")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics, "no heartbeat yet")
	}, 2*time.Minute, 15*time.Second, "agent heartbeat not found")
}
```

Put a config like this next to the test (`e2ectl-kind.yml`), then
`start` / `install` / `test --suite <your dir>` — same as above:

```yaml
schema: 1
environment:
  base: kind
  fakeintake: true
agent:
  version: "7.69.0"   # or: source: true (build from the checkout), pipeline: N (CI DEB)
```

The attached environment gives the suite its components: `FakeIntake.Client()`
queries what the Agent sent, `KubernetesCluster` is a client-go client, and
host environments provide `RemoteHost` for command execution (SSH on a VM,
`docker exec` in a container — same interface). For a host-style test use
`environments.Host` and `--base local`.

State lives in `$E2ECTL_HOME` (default `~/.e2ectl`), one directory per
environment; the snapshot there is what every command reattaches from.

---

## For contributors

### Discover available environment types

```sh
e2ectl environments
e2ectl environments --json
```

This lists the environment **types registered in the CLI**, their descriptions,
the default agent source each installs when none is selected, and the
(deterministically derived) installers each base supports. An installer marked
`(update)` implements the update capability. It does not query cloud accounts
or inspect existing infrastructure.

`e2ectl list` is different: it lists the **instances you have already created** in the
local environment store (`E2ECTL_HOME`, default `~/.e2ectl`).

### Generate a starter configuration

```sh
# Print annotated YAML to stdout:
e2ectl init --base kind

# Create a new private file:
e2ectl init --base kind --output my-kind.yaml
e2ectl init --base ec2-host --output my-vm.yaml
```

The default output is stdout; `--output -` selects it explicitly. File output refuses
to overwrite an existing path, including symlinks. There is no overwrite/force option.

Generation is offline: it needs no credentials, environment store, Docker, kind or
Pulumi executor. It does not deploy anything. The generated YAML comes from annotated Go config structs and is validated against
the registered schema, optional semantic hooks and installer. It contains editable
example versions—not a live snapshot or credentials. Examples are not runtime defaults. Review the comments and choose your desired versions,
architecture and instance size before starting an environment.

The `agent:` section is the simplified installation surface: pick at most ONE
source — `source: true` (build from this checkout), `pipeline: N` (the CI
pipeline's DEB artifacts) or `version: "X"` (a released agent) — or none, which
selects the base's default (`source: true` on local, the pinned released
version on kind, eks and the host bases). The install mechanism and the artifact build
provider are **derived** from (source, base) by a pure function in
`cmd/internal/envconfig/agent`; the installer-owned sections and build
providers still exist but are internal, never user-written. Common fields work
on every mechanism: `config` (extra datadog.yaml), `integrations` (conf.d
folder -> contents), and `values` (extra Helm chart values, the Helm bases
kind and eks).
Unsupported source/base combinations are rejected with a pointer to the
alternatives. Old configs with `install:`/`build:` blocks are rejected with a
pointer to the new shape — a deliberate breaking change, no dual format.
Derivation happens at parse time, so `start` may use an infrastructure-only
config (`agent: {}`).

For kind, `environment.kind.version` selects the Kubernetes `kindest/node` image, not
the version of the kind CLI installed on your machine. `nodes` counts extra workers,
in addition to the control-plane node.

Runtime prerequisites still apply when you actually provision or install. Kind needs
local Docker and kind. Cloud bases (EC2, EKS, Docker) need the configured Pulumi/AWS
environment and a matching executor binary; the EKS endpoint is private, so installs
and tests additionally need the runner-profile VPN up. Agent installation reads credentials from the existing runner profile
(`~/.test_infra_config.yaml` or `E2E_API_KEY`; Helm also reads `E2E_APP_KEY`). Credentials
are deliberately not generated into the starter configuration.

### When a start fails

A failed `start` marks the environment `error` — it never stays stuck in `provisioning` —
and `e2ectl list` shows the truth. `e2ectl stop` recovers the entry: kind removes a
half-created cluster best-effort (clusters are named after the environment) and reports
a warning if one may remain; EC2 runs the normal teardown. If teardown itself fails
(e.g. the executor cannot destroy a stack that was never created), `e2ectl stop --force`
removes the entry anyway and warns which resources may remain to check manually
(kind clusters are named after the environment; EC2 stacks are named `e2ectl-<name>`).
The name is reusable once the entry is removed.

### Build

From the repository root:

```sh
# The core is sufficient for discovery, config generation and local kind operations.
bazel build //test/e2e-framework/cmd/e2ectl:e2ectl

# Also build the executor for EC2 provisioning.
bazel build //test/e2e-framework/cmd/e2ectl-worker:e2ectl-worker
```

Use `bazel cquery <target> --output=files` to locate the built executable. Place the
executor beside the core binary when using the cloud (EC2/EKS/Docker) bases, or set `E2ECTL_WORKER` explicitly.
Building a Bazel target does not replace a separate `./e2ectl` copy automatically.

### Adding an environment type

1. Declare a data-only config type and schema in `cmd/internal/envconfig/<type>`, shared
   with the executor when using Pulumi. Annotate fields with names, defaults, examples,
   constraints and descriptions.
2. Implement the typed `driver.Implementation[Config]` lifecycle and installers.
   `ID()` and `Description()` remain required; no handwritten YAML or mandatory
   `Validate` method is needed. A base provisioned by the Pulumi executor embeds
   the shared lifecycle in `internal/drivers/pulumiworker` (provision, fakeintake
   bookkeeping, destroy) and adds only its differences — see `drivers/dockerhost`
   for the smallest example and `drivers/eks` for one with a post-provision
   kubeconfig export; the executor side is one builder function in
   `cmd/e2ectl-worker/scenarios.go`. An installer declares its own typed agent section in
   `cmd/internal/envconfig/<installer>` (validated by the same schema engine) and
   implements `Artifact` and `Validate` over it — the section is internal, the
   derivation produces it.
3. Register explicitly in `internal/driver/registry.go`, for example:

   ```go
   Define(dockerconfig.Schema, "script", dockerhost.New())
   ```

4. Teach the derivation table in `cmd/internal/envconfig/agent/derive.go` the new
   base: which mechanism each source selects, which combinations are rejected,
   and the base's default source (`DefaultSource`, `Example`). Without this step,
   configs on the new base fail derivation with the supported alternatives.

`Define` prepares a typed configuration before execution and generates examples from
its schema. The driver receives that prepared value rather than decoding raw YAML again.
The second argument is an explicit default installer; starter generation validates
through the installer the derivation selects for the base's default source.

Optional `Validate(params Config) error` is detected on the driver and runs after
automatic validation/defaulting. Put semantic rules that must also run in the executor
in a shared `configschema.Validator[Config]` registered with the shared schema instead.
Both forms must be pure: `init` also validates examples, without runtime credentials or
network access. Availability and authentication checks stay in execution.

See [`cmd/internal/configschema/README.md`](../internal/configschema/README.md) for the
annotation vocabulary, optional validators and normalization rules. The generated active
configuration must pass schema and installer validation; required fields without an
example/default/selector override produce an explicit generation error.

The executor protocol also carries normalized common fixture options separately from
provider parameters, preserving `fakeintake: false`. The CLI checks the executor protocol
before provisioning, so rebuild **both binaries** after protocol changes.

## Receivers and artifact sources

The installer consumes a prepared artifact; the user selects only the agent
**source** and the derivation picks the (internal, typed) artifact provider.
Receivers are explicit routing intent, not permission to trust arbitrary bytes
or to reconfigure fixtures.

| Agent source / base | Derived mechanism (internal) | Managed receiver boundary |
|---|---|---|
| `source: true` / local | `binary` from `invoke-binary` (checkout build) | Core-source capability receipt and verified native runtime required |
| `source: true` / kind | `helm` from `invoke-image` (hacky dev image, deterministic tag, pinned base) | Tested 7.83.0-base source receipt; DCA stays separately released 7.83.0 |
| `source: true` / ec2-host, docker-host | rejected: remote source builds not automated — use pipeline or version | — |
| `source: true` / eks | rejected: no remote image delivery yet — use version for a released chart | — |
| `pipeline: N` / local | `package` from the `pipeline` download provider (exact DEB, receipt-pinned digest) | Explicit receiver required, bounded core-health/configuration scope, no producer attestation |
| `pipeline: N` / kind | rejected: pipeline images not downloadable yet — use version or source | — |
| `pipeline: N` / eks | rejected: pipeline images not downloadable yet — use version for a released chart | — |
| `pipeline: N` / ec2-host, docker-host | `package` from the `pipeline` download provider (exact DEB) | Explicit receivers require a verified producer capability receipt (pipeline DEBs carry none) |
| `version: X` / kind, eks | `helm` (chart version X) | Released managed profile limited to 7.83.0 |
| `version: X` / ec2-host, docker-host | `script` (install script version X) | Released managed profile limited to 7.83.0 |
| `version: X` / local | rejected: released versions install on the cluster (Helm) and host (install script) bases — use source or pipeline | — |

The `existing-*` and `omnibus-repackage` providers remain registered and
receipt-tested but are no longer user-selectable: no source derives them.

### Managed blackhole sink on local

`receiver: type: blackhole` with nothing else selects the **environment-managed
sink**: the local base starts a blackhole container on the environment's Docker
network (the running `e2ectl` binary mounted read-only into the pinned agent
image, unprivileged, read-only, no host port), computes its in-network endpoint
(`<env>-blackhole:8080`), and routes the Agent to it — no manual sink container,
no hand-copied DNS name.

```yaml
agent:
  source: true
  receiver:
    type: blackhole
```

The sink exists only while an Agent is routed to it: `install`, `update` and
`receiver apply` reconcile it before the Agent container starts; switching the
selection away removes it, and `stop` always cleans up the deterministic
container name. `receiver plan` stays read-only: without a running sink it
reports the missing managed sink instead of creating one. Setting
`blackhole.url` keeps the previous bring-your-own-sink behavior, and remote
bases (kind, EC2) do not manage a sink — their explicit Agents must bring one
via `url`.

### Unified `package` installer

`package` installs a verified DEB on whichever target the environment snapshot
attaches: a Docker-transport host (the local base, before its first install) is
the **container target**; an SSH host is the **host target**. The core-health
vs full-systemd limitation is a property of the target, not of the installer:
local Docker targets run the core Agent in the foreground only; remote VM
targets install through apt with full systemd services.

On the container target, `package` installs the full, explicitly SHA256-selected
DEB with real `dpkg` inside an Ubuntu 24.04 Docker filesystem, then starts only
the core Agent in the foreground on the existing local environment's network.
Preparing this runtime image is package **installation**, not a source build —
only `existing-package` is accepted there. The local Docker daemon must already
contain `docker.io/library/ubuntu:24.04` for the native architecture.

The container target does **not** create a `ProducerProfile`, authorize the SSH
package path, or claim all-signal routing safety. It validates neither
systemd/services nor subagents, nor backend ingestion. Its config is the standard
receiver-validated `datadog.yaml` overlay (raw destinations, credentials and
unsupported backend sections are rejected); re-enabling the settings the core-only
container target force-disables is rejected instead of silently overridden. The
host target accepts the same config plus `integrations`; the container target
runs the fixed Go core checks and rejects integrations. An explicit receiver is
required on the container target—no legacy fallback; the host target keeps the
legacy plan fallback.

```yaml
agent:
  pipeline: 138372337   # the pipeline whose DEB is downloaded and installed
  config: |
    tags: [purpose:local-core-health]
  receiver:
    type: fakeintake
    fakeintake:
      remote-config: disabled
```

The pipeline download provider fetches the exact DEB through the repository's
`dda inv package.download` task, computes its SHA256 itself and pins it in the
artifact receipt: no user-typed digest, and nothing left unpinned. The
container installation verifies that digest during installation, and the SSH
upload verifies the same value again on the remote host. The download needs
`dda` on PATH and network access to the testing bucket.

Installation on the container target sets `policy-rc.d` **before** package
scripts run. A credential-free preparation layer installs Ubuntu's standard
`ca-certificates` using apt; the separate DEB/maintainer-script layer explicitly
disables networking. There is no service manager shim, host `/opt` mount, Docker
socket, or privileged container. No credentials enter image layers and TLS
verification is never bypassed. Before resolving a native runner key, an
isolated, dummy-key probe runs the installed core, health and CPU check, and
verifies effective owned endpoints and disabled features through `agent config`.
The live process is checked again before publishing success. `_agent_package_core`
records the archive/core-executable checksums, base/runtime image identities and
checked non-secret settings; receiver status names the container-target scope
`core-health/configuration` and keeps delivery `unverified`.

Fakeintake↔blackhole receiver-only apply reuses and verifies the installed archive,
image and owned runtime volume. It performs no package/source build. Native RC
policy/site/key transitions remain rejected; use a fresh environment, never wipe
RC caches. `stop` removes the Agent, owned volume, fixture and network. Docker's
installation image/cache can remain for reuse; remove exact unused image IDs
separately if desired, not global Docker caches.

For the unchanged local health test, use the retained configs under
`test/new-e2e/tests/agent-subcommands/validation/` and run only:

```sh
E2ECTL_ENV=<name> E2ECTL_HOME=<private-store> dda inv -- new-e2e-tests.run \
  --targets=./tests/agent-subcommands/ \
  --run='^TestLinuxHealthSuiteOnLocal$/^TestDefaultInstallHealthy$' --timeout=3m
```

Health PASS is not ingestion proof—or even proof of successful HTTPS forwarding.
For native acceptance, separately observe the Agent's forwarder success/error
counters without persisting raw credential-bearing status. HTTP acceptance still
is not an independent indexed-metric query. Do not select the other health subtest: it
requests an EC2 update. For a native destination use the real runner key on CLI
install/apply; test-harness-only dummy keys must not override that Agent credential.

### Local image iteration

`source: true` on kind builds the hacky development image from the checkout
(the invocation root), tags it deterministically
(`localhost/datadog-agent:7.83.0-e2ectl-dev`, on the pinned 7.83.0 base) and
loads it into the cluster:

```yaml
agent:
  source: true
  values: |
    agents:
      customAgentConfig:
        log_level: debug
  receiver:
    type: fakeintake
    fakeintake: {remote-config: disabled}
```

The internal `invoke-image` provider receives defaulted repository, reference
and base-image, so none of them appear in user config. Existing images and
packages **never build**; the existing binary path builds from the invocation's
repository root. Released script/version paths retain their behavior.
`--skip-build` verifies installed pins or errors; it never silently acquires
new source outputs. The `existing-image` reuse path (consuming an exported
receipt without compiling) is no longer user-selectable — it remains an
internal, receipt-tested provider.

New binary runtime state uses a private, owned Docker volume; normal stop removes
only that exact volume after stopping the Agent. Meaningful intermediate host-bind
state requires explicit migration/recreation. Unprofiled receipts cannot enter
managed routing via install, reuse or receiver apply; rebuild explicitly rather
than editing profile metadata. `stop --force` does not repair root-owned host paths.

`receiver apply` is Binary-only and reuses its attested pins/runtime; it is not an
install/update fallback. Capture/sink routing uses dummy credentials, but readiness
is not ingestion evidence and routing is not a whole-environment egress sandbox.
Package failures can leave owned runtime masks for repair; no automatic rollback
or state migration is promised.

- [Receiver integration guide](../../testing/receivers/README.md): supported
  signals, diagnostic exclusions, native/fakeintake/blackhole examples, capability
  contracts and truthful state.
- [Artifact integration guide](../../testing/installers/agentbuild/README.md): all
  source-selection YAML, exact receipt/runtime dependencies, isolated Omnibus
  safety, package verification scope, compatibility and extension recipe.
