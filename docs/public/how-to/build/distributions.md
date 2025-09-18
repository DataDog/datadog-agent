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

Omnibus is best called indirectly, via [dda](../../setup/required.md/#tooling) commands.
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
    datadog/agent-buildimages-linux
    dda inv -- -e omnibus.build --base-dir=/omnibus --gem-path=/gems
```

/// info
The container will bind-mount 3 volumes on the host to avoid starting from scratch
at each Omnibus run:

 * `/tmp/omnibus`, containing the Omnibus base directory
 * `/tmp/opt/datadog-agent`, containing the Omnibus installation directory
 * `/tmp/gems`, containing all the ruby gems installed with Bundler
///

### Building on the host

/// danger
Building on the host is _not recommended_, and this section of the guide will be maintained on a best-effort basis.

Running Omnibus builds locally may affect the global state of your machine, and in particular the installation of the Agent already present on your laptop.

Please use the [containerized build](#containerized-build-recommended) instead.
///

Running an Omnibus build will both create and install an Agent distribution package.

* The project will be built locally into a `.tar.xz` archive under `omnibus/pkg`.
* The project will be installed under `/opt/datadog-agent`. This is the same path where the Agent is installed on customer machines.

/// warning
If you already have a Datadog Agent installed, you will need to move it to a different location before operating Omnibus - otherwise it will get overwritten by the build.
> As a Datadog employee, an Agent is installed on your machine during IT's onboarding session.
///

??? note "Linux-specific requirements"
    * On Linux, you will need root privileges, as you need permission to write into `/opt`
    * On Linux, some configuration files will also be dropped under `/etc`.

1. Follow the [general local setup instructions](../../setup/manual.md)
1. Make `/opt` world-readable
1. Run the following command:
```
dda inv -- omnibus.build --base-dir=$HOME/.omnibus
```
> On MacOS, you might want to skip the signing step by adding the `--skip-sign` flag.

The path you pass with the `--base-dir` option will be used as a working directory for the Omnibus build. Once the build completes, it will contain:

| Directory | Contents                                         |
| --------- | ------------------------------------------------ |
| `src`     | The sources downloaded by Omnibus                |
| `cache`   | The binaries cached after building those sources |
| `pkg`     | The final `deb`/`rpm`/`pkg` artifacts            |

??? note "Make sure to pass a `--base-dir` !"
    It is strongly advised to pass a `--base-dir`, and point it to a directory **outside of the Agent repo.**

    By default Omnibus stores packages in the project folder itself: running the task multiple times would recursively add those artifacts to the source files for the `datadog-agent` software definition.


/// tip
You can fine tune an Omnibus run by passing more options, see `dda inv -- omnibus.build --help` for the list of all the available options.

You can chose to generate an installable package in the form of a `deb`/`rpm` artifact by providing a `OMNIBUS_FORCE_PACKAGES` environment variable during the build.
> On macOS, a `dmg` artifact will always be generated.
///

## Building for MacOS

We do not currently support MacOS development environments or any container build image. You will therefore need to follow the [host-based build instructions](#building-on-the-host).

When running the build command, you might want to skip the signing step by adding the `--skip-sign` flag.

## Building for Windows

This can only be done in a containerized environment. Please see [the relevant folder in `datadog-agent-buildimages`](https://github.com/DataDog/datadog-agent-buildimages/tree/main/windows) for more details and instructions.
