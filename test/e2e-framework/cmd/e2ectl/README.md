# e2ectl — QA environments for the Datadog Agent

Spin up a named environment (a local kind cluster, a local Agent container, or
an EC2 VM), install and update the Agent on it independently, see what the
Agent really sent through a fakeintake, and run your tests against the live
environment — no Pulumi for local operations.

## Install

From the repository root:

```sh
bazel build //test/e2e-framework/cmd/e2ectl:e2ectl
bazel cquery //test/e2e-framework/cmd/e2ectl:e2ectl --output=files   # locate it
```

Copy it on your `PATH` as `e2ectl`. You also need Docker running, the `kind`
CLI, and an Agent API key (`~/.test_infra_config.yaml` or `E2E_API_KEY`).

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
e2ectl update --env dev                # rebuild + redeploy, infrastructure untouched
```

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
  install: helm
  helm:
    version: 7.69.0
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
