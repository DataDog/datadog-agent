# e2ectl — QA environments for the Datadog Agent

Create reusable Agent test environments: provision a cluster or a container,
install and update the Agent independently, inspect what it actually sent
through a fakeintake, and run existing E2E tests against the live environment —
without Pulumi for local operations (EC2 provisioning runs in a separate
executor binary).

- **Environments**: a local kind cluster, a local Agent container, or an EC2
  VM — each named, reusable, and inspectable with `e2ectl list`.
- **Agent lifecycle**: install once, then `update` rebuilds your working-tree
  change and redeploys without touching infrastructure.
- **Fakeintake**: every environment ships one; `e2ectl fakeintake` shows the
  metrics the Agent really emitted.
- **Tests**: `e2ectl test` runs a new-e2e suite attached to a live
  environment — the same test bodies as CI, only the provisioning differs.

## Requirements

- Docker (running) and the `kind` CLI — for kind environments.
- The `dda` CLI — only for the local-container Agent install, which builds
  the Agent binary from your working tree.
- An Agent API key: a runner profile at `~/.test_infra_config.yaml`, or
  `E2E_API_KEY` (and `E2E_APP_KEY` for Helm installs) in your environment.
  Credentials are never written into e2ectl configs.

## Installation

From the repository root:

```sh
bazel build //test/e2e-framework/cmd/e2ectl:e2ectl

# Locate the executable (or use the path directly):
bazel cquery //test/e2e-framework/cmd/e2ectl:e2ectl --output=files
```

Copy it somewhere on your `PATH`, or invoke it by path. The commands below
assume an `e2ectl` on `PATH`. EC2 environments additionally need
`//test/e2e-framework/cmd/e2ectl-worker:e2ectl-worker` built and placed beside
the core binary (or `E2ECTL_WORKER` set to its path).

## Quickstart: an environment with the Agent installed

Five commands, ~5 minutes:

```sh
# 1. What environment types exist?
e2ectl environments

# 2. Generate an annotated starter config (review the comments — versions
#    and sizes are examples, not defaults):
e2ectl init --base kind --output my-kind.yaml

# 3. Create the environment (a kind cluster + a local fakeintake):
e2ectl start --config my-kind.yaml --name dev

# 4. Install the released Agent on it (Helm chart):
e2ectl install --env dev

# 5. See what the Agent sent:
e2ectl fakeintake names --env dev        # metric names
e2ectl fakeintake metrics --env dev --name system.cpu.user
```

Other useful commands:

```sh
e2ectl list                    # your environments and their status
e2ectl fakeintake health --env dev
e2ectl update --env dev        # rebuild your Agent change and redeploy
e2ectl stop --env dev           # destroy everything (cluster, fakeintake, entry)
```

Instead of a released Agent, `agent.helm.image` installs a locally built
image, and `--base local` runs the Agent binary itself in a container on a
private Docker network — the fastest iteration loop, with the same config
surface.

## The iteration loop

The point of e2ectl: change Agent code, redeploy it, see the difference —
infrastructure untouched:

```sh
# ...edit Agent Go code...
e2ectl update --env dev                 # rebuild + redeploy
e2ectl fakeintake metrics --env dev --name my.renamed.metric
```

## Running a test against your environment

`e2ectl test` runs an existing new-e2e suite attached to a live environment —
it resolves the environment, verifies it is ready with the Agent installed,
exports the attach variables, and dispatches to `go test`:

```sh
e2ectl test --env dev \
  --suite ./test/new-e2e/tests/agent-subcommands/ \
  --run 'TestLinuxHealthSuiteOnLocal/TestDefaultInstallHealthy'
```

Attachable entry points follow the naming convention `<Test>On<Local|Host>`
and skip themselves unless `E2ECTL_ENV` is set — so the same suite keeps
working in CI (provisioned) and locally (attached). Complete examples live in
the tree, config file next to the test:

| Suite | Config | Entry point |
|---|---|---|
| agent-subcommands health | `tests/agent-subcommands/e2ectl-local.yml` | `TestLinuxHealthSuiteOnLocal` |
| containers kindSuite (the original) | `tests/containers/e2ectl-kind.yml` | `TestKindSuiteOnLocalKind` |

```sh
e2ectl start  --config test/new-e2e/tests/containers/e2ectl-kind.yml --name my-kind
e2ectl install --env my-kind
e2ectl test --env my-kind --suite ./test/new-e2e/tests/containers/ -run TestKindSuiteOnLocalKind
```

## Writing your own test

A test attaches with three calls from the `e2ectlenv` helper; the body is
plain new-e2e. Put the e2ectl config next to the test file.

`mycheck/e2ectl-kind.yml` — the environment your test needs:

```yaml
schema: 1
environment:
  base: kind
  fakeintake: true
agent:
  install: helm
  helm:
    version: 7.69.0
```

`mycheck/mycheck_test.go` — the test:

```go
package mycheck

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	e2ectlenv "github.com/DataDog/datadog-agent/test/new-e2e/utils/e2ectlenv"
)

// mySuite works on any Kubernetes environment with an installed Agent.
type mySuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestMyCheckOnLocal attaches to a running e2ectl environment
// (e2ectl test sets E2ECTL_ENV); the body is the same as any new-e2e suite.
func TestMyCheckOnLocal(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)          // skips when E2ECTL_ENV is unset
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &mySuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Kubernetes](envName),
	))
}

// TestHeartbeat asserts the Agent is alive and flushing to the fakeintake.
func (s *mySuite) TestHeartbeat() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.agent.running")
		require.NoError(c, err, "failed to filter metrics")
		assert.NotEmpty(c, metrics, "no heartbeat metric yet")
	}, 2*time.Minute, 15*time.Second, "agent heartbeat not found in fakeintake")
}
```

Run it:

```sh
e2ectl start  --config test/new-e2e/tests/mycheck/e2ectl-kind.yml --name my-dev
e2ectl install --env my-dev
e2ectl test --env my-dev --suite ./test/new-e2e/tests/mycheck/
```

The environment gives the suite its components through the snapshot:
`FakeIntake.Client()` queries what the Agent sent, `KubernetesCluster` is a
client-go client on the cluster, and on host environments `RemoteHost`
executes commands (SSH on a VM, `docker exec` in a local container — the same
interface). For a host-style test use `environments.Host` in `Attach[...]`
and `--base local` in the config.

## Where state lives

`$E2ECTL_HOME` (default `~/.e2ectl`), one directory per environment:
the snapshot (the single source of truth every command reattaches from),
the kubeconfig, the stored config copy. `e2ectl list` reads it; deleting an
environment directory is equivalent to `stop --force`.

---

## For contributors

### Discover available environment types

```sh
e2ectl environments
e2ectl environments --json
```

This lists the environment **types registered in the CLI**, their descriptions and
supported installers. An installer marked `(update)` implements the update capability.
It does not query cloud accounts or inspect existing infrastructure.

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

The `agent:` section mirrors the `environment:` section: `install` selects the installer,
and the section named after it (`script:`, `helm:`) is that installer's typed config.
Fields an installation method does not consume do not exist there — an `image` cannot
be written for a script install, and unknown keys fail with a pointer to the right
section. Installer sections live in `cmd/internal/envconfig/{script,helm}`, next to the
environment schemas. Agent section contents are validated when the installer runs
(install/update), so `start` may use an infrastructure-only config.

For kind, `environment.kind.version` selects the Kubernetes `kindest/node` image, not
the version of the kind CLI installed on your machine. `nodes` counts extra workers,
in addition to the control-plane node.

Runtime prerequisites still apply when you actually provision or install. Kind needs
local Docker and kind. EC2 needs the configured Pulumi/AWS environment and a matching
executor binary. Agent installation reads credentials from the existing runner profile
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
executor beside the core binary when using EC2, or set `E2ECTL_WORKER` explicitly.
Building a Bazel target does not replace a separate `./e2ectl` copy automatically.

### Adding an environment type

1. Declare a data-only config type and schema in `cmd/internal/envconfig/<type>`, shared
   with the executor when using Pulumi. Annotate fields with names, defaults, examples,
   constraints and descriptions.
2. Implement the typed `driver.Implementation[Config]` lifecycle and installers.
   `ID()` and `Description()` remain required; no handwritten YAML or mandatory
   `Validate` method is needed. An installer declares its own typed agent section in
   `cmd/internal/envconfig/<installer>` (validated and example-generated by the same
   schema engine) and implements `AgentExample`, `Artifact` and `Validate` over it.
3. Register explicitly in `internal/driver/registry.go`, for example:

   ```go
   Define(ec2config.Schema, "script", &ec2host.Driver{})
   ```

`Define` prepares a typed configuration before execution and generates examples from
its schema. The driver receives that prepared value rather than decoding raw YAML again.
The second argument is an explicit default installer for full-config generation.

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
