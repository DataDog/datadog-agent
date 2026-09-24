# Build image reference

-----

All non-macOS release artifacts are produced within [container images](https://github.com/DataDog/datadog-agent-buildimages) that are hosted on the [Datadog Docker Hub](https://hub.docker.com/u/datadog).

/// note
These images are not meant to be used directly outside of CI scenarios. Instead, use the [developer environments](dev.md) to build and test the Agent.
///

## Linux

The Linux [builder](https://github.com/DataDog/datadog-agent-buildimages/tree/main/linux), available as [datadog/agent-buildimages-linux](https://hub.docker.com/r/datadog/agent-buildimages-linux), is used to build all Linux release artifacts except for the RPM distribution (temporary limitation). It ships a custom toolchain for cross-compiling the Agent.

The image is based on Ubuntu 24.04 and supports both the `amd64` and `arm64` host architectures.

## Windows

The Windows [builder](https://github.com/DataDog/datadog-agent-buildimages/tree/main/windows), available as [datadog/agent-buildimages-windows_x64](https://hub.docker.com/r/datadog/agent-buildimages-windows_x64), is used to build all Windows release artifacts.

The image is based on Windows Server 2022 and only supports the `amd64` host architecture.

## macOS

Due to licensing restrictions, we are unable to maintain a dedicated macOS container image. macOS release artifacts are built on a custom AMI that is not publicly available. Contributors must [manually](../../setup/manual.md) set up their local environment in order to build macOS-specific release artifacts.
