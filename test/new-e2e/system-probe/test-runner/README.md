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
can define a `test_env_vars` map. The test-runner matches the micro-VM's
`uname` architecture and kernel release against the platforms list and
exports every entry as an environment variable for the test binaries, with
its platforms.json key as the variable name. For example, the
`DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED` entry gates the `Fentry` build mode
in `pkg/ebpf/ebpftest.SupportedBuildModes`. When no platform matches the
running kernel (e.g. custom kernels), no env vars are set.
