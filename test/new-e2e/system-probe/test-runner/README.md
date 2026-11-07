# About
This is a helper utility of the KMT framework.

As the name suggests, this pkg is used to execute all system-probe UTs **inside** micro-vms. It is compiled on-the-fly by KMT framework.
It is leveraging the [gotestsum](https://github.com/gotestyourself/gotestsum) binary to actually execute the UTs.

`test-runner` helper is executed as part of [micro-vm-init.sh](../test/micro-vm-init.sh), whereas the script is copied and executed inside each micro-VM via `kmt.test` invoke task.

The results of the tests (including the junit summary file) are stored in the `/ci-visibility` directory on the target micro-VM.
See `buildTestConfiguration` in main.go of this package for the list of different parameters supported by this test-runner

## Platform test env vars

Each KMT platform in
[test/new-e2e/system-probe/config/platforms.json](../config/platforms.json)
can define a `test_env_vars` map. The CI job that provisions the micro-VM
passes the platforms.json architecture and key of the booted image to the VM
through `job_env.txt` (see `.gitlab/test/kernel_matrix_testing/common.yml`),
and the test-runner looks up that entry and exports every one of its
`test_env_vars` as an environment variable for the test binaries (the map
key is the variable name). For example, the
`DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED` entry gates the `Fentry` build mode
in `pkg/ebpf/ebpftest.SupportedBuildModes`. The platform is never inferred
from the running kernel: when no key is provided (e.g. local runs or custom
kernels) no env vars are set, and the test binaries fall back to their own
runtime detection. A provided key that is missing from platforms.json is an
error.
