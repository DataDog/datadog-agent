# How to build agent distribution packages

---
## Intro
### What are distribution packages ?

By "distribution package", we mean an artifact used by end users to install the Agent on their system. The format is OS-specific:


| OS      | Package Format                              |
| ------- | ------------------------------------------- |
| Linux   | `.deb` (Debian-based) / `.rpm` (RHEL-based) |
| Windows | `.msi`                                      |
| macOS   | `.pkg`                                      |


These distribution packages contain [a binary of the Agent](standalone.md), along with any supporting libraries needed for the agent to function properly.

### Omnibus

Agent packages for all the supported platforms are built using [Omnibus](https://github.com/chef/omnibus).
/// info
There is an ongoing effort to migrate our build system to [Bazel](https://bazel.build/), so this may change in the near-to-mid future.
///

/// warning
Omnibus creates a package **for the operating system it runs on**, so you'll get a `.deb` package on Debian-based distros, an `msi` installer on Windows etc.

There is currently no way to "cross-build" packages for a platform different than the host's.
///

Omnibus is best called indirectly, via [dda](../../../setup/required/#tooling) commands.
The main entrypoint is the `omnibus.build` invoke task, which you can run like this:
```bash
dda inv omnibus.build
```
> This will probably not work out of the box though - see instructions below for more information.


## Building for Linux: `deb` and `rpm`
### Containerized build (recommended)

We provide a Docker image containing all the build dependencies required for building Omnibus Linux packages: [`datadog/agent-buildimages-linux`](https://hub.docker.com/r/datadog/agent-buildimages-linux)

??? tip "Building the image locally"
    The Dockerfile for this image is available in the [datadog-agent-buildimages](https://github.com/DataDog/datadog-agent-buildimages) repository.
    To build it from scratch, you can run the following command from the root of that repo:

    ```bash
    docker build -t datadog-agent-buildimages:linux -f linux/Dockerfile .
    ```

1. Make sure Docker is running on your machine.
1. Navigate to the root folder of a clone of the `datadog-agent` repo.
1. Run the following command, which will create a container for the previously-mentioned image, and run the `omnibus.build` task inside.
```bash
docker run
    -v "$PWD:/go/src/github.com/DataDog/datadog-agent"
    -v "/tmp/omnibus:/omnibus"
    -v "/tmp/opt/datadog-agent:/opt/datadog-agent"
    -v"/tmp/gems:/gems"
    --workdir=/go/src/github.com/DataDog/datadog-agent
    datadog/agent-buildimages-linux-glibc-2-17-x64
    dda inv -- -e omnibus.build --base-dir=/omnibus --gem-path=/gems
```

/// info
The container will bind-mount 3 volumes on the host to avoid starting from scratch
at each Omnibus run:

 * `/tmp/omnibus`, containing the Omnibus base directory
 * `/tmp/opt/datadog-agent`, containing the Omnibus installation directory
 * `/tmp/gems`, containing all the ruby gems installed with Bundler
///

///bug
If the build image crashes when running it on a modern Linux distribution, you might be
affected by [this bug](https://github.com/moby/moby/issues/28705).
///

