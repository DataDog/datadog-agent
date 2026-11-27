# Running E2E tests

Complete the [one-time setup](index.md#one-time-setup) before running anything on this page.

## Basic Test Execution

E2E tests are located in the `test/new-e2e/` directory. After running `dda inv e2e.setup` once, you can run them like unit tests — no `aws-vault exec` wrapping, no exported `PULUMI_CONFIG_PASSPHRASE`. The runner reads the passphrase from your local config and auto-wraps the test command with `aws-vault exec` against the configured profile.

```bash
# Run a simple VM test
dda inv new-e2e-tests.run --targets=./examples --run=^TestVMSuite$
```

Replace ./examples with your subfolder.
This also supports the golang testing flag --run and --skip to target specific tests using go test syntax. See go help testflag for details.

```bash
dda inv new-e2e-tests.run --targets=./examples --run=TestMyLocalKindSuite/TestClusterAgentInstalled
```

You can also run it with go test, from test/new-e2e
```bash
cd test/new-e2e && go test ./examples -timeout 0 -run=^TestVMSuite$
```

While developing a test you might want to keep the remote instance alive to iterate faster. You can skip the resources deletion using dev mode with the environment variable `E2E_DEV_MODE`. You can force this in the terminal
```bash
E2E_DEV_MODE=true dda inv -e new-e2e-tests.run --targets ./examples --run=^TestVMSuite$
```
or for instance add it in the `go.testEnvVars` if you are using a VSCode-based IDE
```
"go.testEnvVars": {
  "E2E_DEV_MODE": "true",
}, 
```

## Test with Local Agent Packages

By default a run installs the Agent from the packages of a CI pipeline for your commit, or the
latest nightly when there is none, so code that hasn't been through CI isn't what gets tested.
`--local-package` installs a package you built yourself instead: the framework copies it to the
host and installs it with the package manager (`apt-get install ./<file>.deb`; `yum`, `dnf` or
`zypper` for RPMs; the MSI installer on Windows). It skips the install script and package
repositories, and it doesn't check signatures, so an unsigned local build works.

### 1. Build the package

Bazel is the recommended way: it builds a complete package from your checkout, without root, and
without touching `/opt/datadog-agent` on your machine.

```bash
export PACKAGE_VERSION="$(dda inv -- agent.version --url-safe)"   # otherwise the version is <milestone>~localbuild
bazel build //packages/agent/linux:debian    # or //packages/agent/linux:rpm
mkdir -p /tmp/e2e-pkg/pkg
cp -L bazel-bin/packages/agent/linux/datadog-agent_*.deb /tmp/e2e-pkg/pkg/
```

Build it on Linux (a developer environment or a workspace). With the remote cache it takes a few
minutes. Bazel builds for the machine it runs on, so an arm64 machine, such as an arm64
workspace, produces an `arm64` package.

/// admonition | Bazel package compared to the shipped package
    type: info

Release packages are still built by omnibus. The Bazel package (`//packages/agent/linux:debian`)
installs and runs the same product, and the `bazel:package:linux-amd64` CI job builds and checks
it, but there are known gaps: binaries aren't stripped, so it is more than twice the size of the
omnibus package; `agent version` reports `Commit: dev`; it has no Debian/Red Hat specific
contents yet. None of this affects functional tests, so prefer it for E2E runs. Don't use it
to judge package size, and for packaging changes that only omnibus applies, use the
omnibus path below or a CI pipeline.
///

The omnibus alternative replaces the binaries in the latest nightly package with your local
build. It only produces DEB packages, overwrites `/opt/datadog-agent` on the machine it runs on
(use a [developer environment](../../../tutorials/dev/env.md)), and it can't be used to test
packaging or Python integration changes:

```bash
dda inv omnibus.build-repackaged-agent     # then pass --local-package $(pwd)/omnibus
```

### 2. Run the test with the package

`--local-package` takes a file, or a directory with the package under `pkg/` (`suse/pkg/` for
SUSE). A directory may hold FIPS and non-FIPS packages: the one matching the Agent flavor is
picked, and `-dbg` packages are ignored. Blank `E2E_PIPELINE_ID` so the run doesn't also pick up
a pipeline's artifacts for your commit:

```bash
E2E_PIPELINE_ID="" dda inv new-e2e-tests.run \
  --targets=./tests/agent-subcommands/status --run='^TestLinuxStatusSuite$' \
  --local-package=/tmp/e2e-pkg
```

The run log shows `Found local package to install <path>` for each host it provisions.

### Match the host to the package

The package architecture must match the test host. The default host is Ubuntu x86_64, so an
`arm64` package fails with `no package found ... with architecture x86_64 or amd64`.

/// admonition | `ddinfra:osDescriptor` doesn't change a suite's host
    type: warning

`--configparams ddinfra:osDescriptor=debian:12:arm64` only applies to `dda inv aws.create-vm`.
Test suites build their host from the options in the test code
(`awshost.Provisioner(...)`), so to run a suite on another OS or architecture, edit those
options locally (don't commit the edit) on **every** provisioner call, including each
`UpdateEnv`, which starts again from the defaults:

```go
var debianArm64 = ec2.WithEC2InstanceOptions(ec2.WithOSArch(e2eos.Debian12, e2eos.ARM64Arch))

e2e.Run(t, &mySuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(debianArm64))))
v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(debianArm64, ec2.WithAgentOptions(...))))
```
///

### Which tests can use a local package

A local package is used by suites that let the framework install the Agent on a VM
(`awshost.Provisioner` and the other host provisioners, with their default Agent version). That
covers most suites that test Agent behavior on a host: subcommands, configuration, checks,
fakeintake payloads, and so on.

It is ignored, or can't apply, in these cases:

| Suites | Why | What to use instead |
|---|---|---|
| Install script, upgrade and step-by-step tests (`tests/agent-platform/tests/`) | They provision the VM without an Agent and install it themselves, from the pipeline's repository | A CI pipeline (`--pipeline-id`) |
| Installer and Fleet tests (`tests/installer/`, `tests/fleet/`) | They install packages through the installer, from the pipeline's OCI artifacts | A CI pipeline |
| Suites that pin a version in code (`agentparams.WithPipeline`, `WithVersion`, `WithLatest`) | The code option overrides the default version | Remove the option locally, or use a pipeline |
| Docker, Kubernetes and ECS suites | They run the Agent image, not a package | `--agent-image` (see [below](#test-with-local-agent-image)) |
| `ddagent:version` set in the config | It takes precedence over `--local-package` | Unset it |
| Suites that require `E2E_PIPELINE_ID` (e.g. `tests/fips-compliance/`) | They pull pipeline-built images and fail fast without one | A CI pipeline, or run the job in the PR's pipeline |

### Validating a test-only change

A change to test code alone, such as switching a suite to `ProvisionerNoFakeIntake` or
removing an `UpdateEnv`, doesn't need a local package, but the Agent version still matters:
a suite can pass with a released Agent and fail with the one the PR's pipeline builds. Run it
both ways:

- Locally, pin the Agent so that the run can be repeated: `-c ddagent:version=<release>` (for
  example the latest stable), or `--pipeline-id <id>` to install the PR pipeline's packages.
  Without either, you get whatever nightly is latest at the time, and the run doesn't record
  which one: on Linux only the `Setting up datadog-agent (...)` line of the install output shows
  it, and Windows doesn't log it.
- In CI, re-run the suite's job in the PR's own pipeline and wait for it to succeed. A suite
  that passes inside a failed job doesn't count, and neither does a local pass. A test that
  passed only after gotestsum re-ran it is a failure to investigate, not a pass. Compare with
  the same test on `main` in CI Visibility before calling a failure flaky.

Separate suites can run in parallel from one worktree each, but each new worktree builds the test
binary with a cold Bazel cache first, which takes about 15 minutes before anything is
provisioned. Each run must also hold valid AWS credentials until teardown, because an expired
SSO session makes Pulumi's `credential_process` time out partway through. If you kill a run,
the stack stays locked: run `pulumi cancel` and `pulumi destroy` on it before running it again.

## Test with Local Agent Image

/// admonition | Limitations
    type: warning

This method relies on updating an existing Agent image with the local Go binaries. It only works for Docker images and must be considered as a solution for testing only.
///

Build the Agent binary and the Docker image, using this command:
```bash
dda inv [--core-opts] agent.hacky-dev-image-build [--base-image=STRING --push --signed-pull --target-image=STRING]
```

The command uses `dda inv agent.build` to generate the Go binaries. The generated image embeds this binary, a debugger and auto-completion for the agent commands.
By default, the image is names `agent` unless you override it with the `--target-image` option.

Then push the image to a registry:
```bash
# Login to ECR
aws-vault exec sso-agent-sandbox-account-admin-8h -- \
aws ecr get-login-password --region us-east-1 | \
docker login --username AWS --password-stdin 376334461865.dkr.ecr.us-east-1.amazonaws.com
# Push the image
docker push 376334461865.dkr.ecr.us-east-1.amazonaws.com/agent-e2e-tests:$USER
```

And finally, execute your E2E tests with the associated command:
```bash
# Run Ubuntu tests
dda inv -e new-e2e-tests.run --targets ./tests/containers \
  --run TestDockerSuite/TestDSDWithUDP \
  --agent-image 376334461865.dkr.ecr.us-east-1.amazonaws.com/agent-e2e-tests:$USER
```

## Monitoring a local run

A run goes through four phases: compiling the test binary (several silent minutes on a cold
cache), provisioning with Pulumi, running the tests, and destroying the stack. These are the
ways to follow it and look at it afterwards.

| What | Where | Notes |
|---|---|---|
| Live console | `--verbose` | Streams the Pulumi resource events (`creating`, `created`, `**failed**`) and the test output, prefixed `multi.go:` |
| Test events as they happen | `e2e_test_output.json.<attempt>.part` at the repository root, while the run is going | `go test -json` lines. `run` events stream live; with testify suites, the subtests' `pass`/`fail` events only arrive when the whole suite finishes |
| Final results | `e2e_test_output.json` and `junit-out-*.xml` at the repository root | Change the paths with `--result-json` and `--junit-tar` |
| Per-test output and diagnostics | `~/e2e-output/<Suite>_<timestamp>/<Test>/`, with `~/e2e-output/latest` pointing at the last run | Change it with `E2E_OUTPUT_DIR` or `configParams.outputDir`. A failed test leaves an Agent flare here (`datadog-agent-<time>-info.zip`) |
| Command telemetry | One `dda` span per command, sent by `dda` | Resource `inv new-e2e-tests.run`, with duration, exit code, the tail of the error output, and who ran it (`exec.actor`: `human`, `claude`, …). No per-test data. `dda self telemetry show log` shows submission errors |

For example, to list test progress from another terminal:

```bash
jq -r 'select(.Test != null and (.Action == "run" or .Action == "pass" or .Action == "fail"))
       | "\(.Time[11:19]) \(.Action) \(.Test) \(.Elapsed // "")"' e2e_test_output.json.0.part
```
