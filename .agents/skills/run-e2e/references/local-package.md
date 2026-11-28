# Testing a local build

`/doc/how-to/test/e2e/running.md` § "Test with Local Agent Packages" is the procedure. This file
adds only the decisions you have to make as an agent.

## Decide whether a local package is needed

A local package is needed when the run must exercise code that no CI pipeline has packaged yet:
an unpushed change, or a branch whose packaging jobs haven't run. Without it the task silently
tests something else, either a pipeline found for your commit or a published version, and a
green result says nothing about the change.

It isn't needed when the change is only in the test code (`test/new-e2e/`, `test/e2e-framework/`):
those are compiled into the test binary, not the Agent.

## Check that the suite can use one

Read the suite's provisioner call before building anything.

- **Can use a local package**: a host provisioner (`awshost.Provisioner`,
  `ProvisionerNoFakeIntake`, …) that leaves the Agent version at its default.
- **Can't**: `ec2.WithoutAgent()` / `ProvisionerNoAgent*` (the suite installs the Agent itself:
  install-script, upgrade, step-by-step, installer and fleet suites); an explicit
  `agentparams.WithPipeline`, `WithVersion` or `WithLatest`; container suites, which need
  `--agent-image`. For those, say so and offer a pipeline run (`--pipeline-id`) instead. Don't
  build a package the run won't install.

## Build it with Bazel

Build with `bazel build //packages/agent/linux:debian` (`:rpm` for RPM-based hosts). Use it even
though releases still ship from omnibus: it builds from the working tree without root and
without overwriting `/opt/datadog-agent`, and its contents are close enough for functional
tests. Only use the omnibus repackaging route when the user asks for it, or when the change
is in packaging that only omnibus applies. The package lands in
`bazel-bin/packages/agent/linux/`; copy it (`cp -L`, it's a symlink) into `<dir>/pkg/` and pass
`--local-package=<dir>`. Blank the pipeline ID too: `E2E_PIPELINE_ID=""`.

## Match the host to the package

Bazel builds for the machine it runs on: `uname -m` = `aarch64` gives an `arm64` package. Suites
default to an Ubuntu x86_64 host, so an arm64 package fails during provisioning with `no package
found ... with architecture x86_64 or amd64`. `--configparams ddinfra:osDescriptor=...` does
**not** fix this: suite provisioners ignore it.

To run on a matching host, add `ec2.WithEC2InstanceOptions(ec2.WithOSArch(<descriptor>,
e2eos.ARM64Arch))` to every provisioner call in the suite, including each `UpdateEnv`, which
starts again from the defaults. That's an edit to test code: tell the user, keep it out of
commits, and revert it after the run.

## Confirm it was installed

The run log prints `Found local package to install <path>` for each host it provisions, and
the remote command name contains the package file name (`apt-install-./datadog-agent_<version>_<arch>.deb`).
Put the package file in the report's `Agent build` line.
