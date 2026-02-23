# Datadog Package fixtures

This directory contains a few examples of Datadog Packages for use in the
updater tests.

*simple-v1*
```bash
datadog-package create --archive --version "v1" --archive-path "pkg/fleet/installer/fixtures/oci-layout-simple-v1.tar" --package "simple" --configs pkg/fleet/installer/fixtures/simple-v1-config --extension simple-extension=pkg/fleet/installer/fixtures/simple-v1-extension pkg/fleet/installer/fixtures/simple-v1
```

*simple-v1-with-extension*
```bash
datadog-package create --archive --version "v1" --archive-path "pkg/fleet/installer/fixtures/oci-layout-simple-v1-with-extension.tar" --package "simple" --extension simple-extension=pkg/fleet/installer/fixtures/simple-v1-extension pkg/fleet/installer/fixtures/simple-v1
```

*simple-v2*
```bash
datadog-package create --archive --version "v2" --archive-path "pkg/fleet/installer/fixtures/oci-layout-simple-v2.tar" --package "simple" --configs pkg/fleet/installer/fixtures/simple-v2-config pkg/fleet/installer/fixtures/simple-v2
```

*simple-v1-linux2-amd128*
```bash
datadog-package create --archive --version "v1" --os "linux2" --arch "amd128" --archive-path "pkg/fleet/installer/fixtures/oci-layout-simple-v1-linux2-amd128.tar" --package "simple" pkg/fleet/installer/fixtures/simple-v1
```
