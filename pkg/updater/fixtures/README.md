# Datadog Package fixtures

This directory contains a few examples of Datadog Packages for use in the
updater tests.

All the packages include the files in `example-package`.

*simple-v1*
```bash
datadog-package create --archive --version "v1" --archive-path "pkg/updater/fixtures/oci-layout-simple-v1.tar" --package "simple-v1" pkg/updater/fixtures/simple-v1
```

*simple-v2*
```bash
datadog-package create --archive --version "v2" --archive-path "pkg/updater/fixtures/oci-layout-simple-v2.tar" --package "simple-v2" pkg/updater/fixtures/simple-v2
```
