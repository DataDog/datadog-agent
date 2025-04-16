# About
This is a helper utility of the KMT framework.

As the name suggests, this pkg is used to execute all system-probe UTs **inside** micro-vms. It is compiled on-the-fly by KMT framework.
It is leveraging the [gotestsum](https://github.com/gotestyourself/gotestsum) binary to actually execute the UTs.

`test-runner` helper is executed as part of [micro-vm-init.sh](../test/micro-vm-init.sh), whereas the script is copied and executed inside each micro-VM via `kmt.test` invoke task.

The results of the tests (including the junit summary file) are stored in the `/ci-visibility` directory on the target micro-VM.
See `buildTestConfiguration` in main.go of this package for the list of different parameters supported by this test-runner
