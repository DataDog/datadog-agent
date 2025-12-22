# How to build agent distribution packages

---
## Intro
### What are distribution packages ?

By "distribution package", we mean an artifact used by end users to install the Agent on their system. The format is OS-specific:


| OS      | Package Format                              |
| ------- | ------------------------------------------- |
| Linux   | `.deb` (Debian-based) / `.rpm` (RHEL-based) |
| Windows | `.msi`                                      |
| macOS   | `.dmg`                                      |


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


## Building for Linux: `deb`
### Using the `dev env`

The [developer environments](../../reference/images/dev.md) we provide contain all the dependencies required for building Omnibus packages, along with extra tools and features that make local development easier.
We recommend that you use them for all your development needs.

To build distribution packages in a developer environment:

1. Follow the [`dev env` tutorial](../../tutorials/dev/env.md) to install the required tools and familiarize yourself with the dev environments.
1. Make sure Docker is running on your system.
1. Start a developer environment, which will automatically pull the latest version of the container image: `dda env dev start`
1. Either:

    * Connect to a shell inside the dev container: `dda env dev shell`
    * Open an IDE window inside the container: `dda env dev code`
    * Run a single command inside the container: prefix the command in the next step with `dda env dev run`

1. Run the following command:
```
dda inv -- -e omnibus.build
```

Once the process completes, the built artifacts will be available _in the container_ under `/omnibus/pkg`.
/// details | Moving the artifacts back to the host
    open: False
    type: tip

The `datadog-agent` local repo clone is bind-mounted into the dev env container. You can use this to access your artifacts from the host, by copying them from `/omnibus/pkg` to `/root/repos/datadog-agent`:
```bash
mv /omnibus/pkg /root/repos/datadog-agent/bin
```
///

### Using the build image

We provide a [Docker image](../../reference/images/builders.md#linux) containing all the build dependencies required for building `deb` packages via Omnibus: [`datadog/agent-buildimages-linux`](https://hub.docker.com/r/datadog/agent-buildimages-linux).This image is the one used by CI, and as such it is quite bare-bones. The developer environments mentioned in [the section above](#using-the-dev-env) are based on this image.

/// details | Building the image locally
    open: False
    type: tip

The Dockerfile for this image is available in the [datadog-agent-buildimages](https://github.com/DataDog/datadog-agent-buildimages) repository.
To build it from scratch, you can run the following command from the root of that repo:

```bash
docker build -t datadog-agent-buildimages:linux -f linux/Dockerfile .
```
///

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

Once the process completes, the built artifacts will be available on your host under `/tmp/omnibus/pkg`.

### Building on the host (discouraged)

/// danger
Building on the host is _not recommended_, and this section of the guide will be maintained on a best-effort basis.

Running Omnibus builds locally may affect the global state of your machine, and in particular the installation of the Agent already present on your laptop.

Please use one of the [containerized build options](#using-the-build-image) instead.
///

Running an Omnibus build will both create and install an Agent distribution package.

* The project will be built locally into a `.tar.xz` archive under `omnibus/pkg`.
* The project will be installed under `/opt/datadog-agent`. This is the same path where the Agent is installed on customer machines.

/// warning
If you already have a Datadog Agent installed, you will need to move it to a different location before operating Omnibus - otherwise it will get overwritten by the build.
> As a Datadog employee, an Agent is installed on your machine during IT's onboarding session.
///

/// details | Linux-specific requirements
    open: False

  * On Linux, you will need root privileges, as you need permission to write into `/opt`
  * On Linux, some configuration files will also be dropped under `/etc`.
///

1. Follow the [general local setup instructions](../../setup/manual.md)
2. Make `/opt` world-readable
3. Run the following command:
```
dda inv -- omnibus.build --base-dir=$HOME/.omnibus
```

The path you pass with the `--base-dir` option will be used as a working directory for the Omnibus build. Once the build completes, it will contain:

| Directory | Contents                                         |
| --------- | ------------------------------------------------ |
| `src`     | The sources downloaded by Omnibus                |
| `cache`   | The binaries cached after building those sources |
| `pkg`     | The final `deb`/`rpm`/`dmg` artifacts            |

/// details | Make sure to pass a `--base-dir` !
    open: False

It is strongly advised to pass a `--base-dir`, and point it to a directory **outside of the Agent repo.**

By default Omnibus stores packages in the project folder itself: running the task multiple times would recursively add those artifacts to the source files for the `datadog-agent` software definition.
///

/// tip
You can fine tune an Omnibus run by passing more options, see `dda inv -- omnibus.build --help` for the list of all the available options.

You can chose to generate an installable package in the form of a `deb`/`rpm` artifact by providing a `OMNIBUS_FORCE_PACKAGES` environment variable during the build.
> On macOS, a `dmg` artifact will always be generated.
///

## Building for Linux: `rpm`

Some extra dependencies are required for building `rpm` packages that are not yet included in the main `datadog/agent-buildimages-linux` build image.

A separate docker image containing these special dependencies is also available. This image, contrary to the main `datadog/agent-buildimages-linux` image, is not multi-arch - thus there are two flavors depending on the CPU architecture of the host machine:

- For `x86_64`/`amd64`: [`datadog/agent-buildimages-rpm_x64`](https://hub.docker.com/r/datadog/agent-buildimages-rpm_x64)
- For `arm64`/`aarch64`: [`datadog/agent-buildimages-rpm_arm64`](https://hub.docker.com/r/datadog/agent-buildimages-rpm_arm64)

To build using these images, follow the same instructions as [for `deb` packages](#using-the-build-image), but replace `datadog/agent-buildimages-linux` with the appropriate flavor of the `rpm` image for your CPU platform.
You can also attempt a [host-based build](#building-on-the-host-discouraged), although this is heavily discouraged.

## Building for MacOS

We do not currently support MacOS development environments or any container build image. You will therefore need to follow the [host-based build instructions](#building-on-the-host-discouraged).

When running the build command, you might want to skip the signing step by adding the `--skip-sign` flag.

## Building for Windows

/// warning
This can only be done in a containerized environment.
Please see [the relevant folder in `datadog-agent-buildimages`](https://github.com/DataDog/datadog-agent-buildimages/tree/main/windows) for more details on the images to use.

/// details | Image naming scheme
    open: False
    type: info

As of the writing of this doc, the relevant images follow this naming pattern: `registry.ddbuild.io/datadog-agent-buildimages/windows_ltsc{$YEAR}_${ARCH}${SUFFIX}:${TAG}`

- `YEAR` is either `2022` or `2025`
- `ARCH` can only be `x64` at this time
- `SUFFIX` can be either empty or `_test_only`, which refers to images used by CI in PR builds.
- The `TAG` follows the usual convention, i.e. `v{gitlab pipeline id}-{short commit sha}`

/// example
`registry.ddbuild.io/ci/datadog-agent-buildimages/windows_ltsc2025_x64:v77240728-510448c3`
///
///
///

First, mount / clone a checkout of the `datadog-agent` repo inside the container.

The recommended way to do this while developing manually is to bind-mount your host's checkout of the repo into the container.On the host, while inside the `datadog-agent` repo:
```powershell
docker run -v "$(Get-Location):c:\mnt" <image>
```

You can then invoke one of the windows build scripts, available in `tasks/winbuildscripts`:

- `Build-AgentPackages.ps1` is used for building the "main" Agent msi package
- `Build-OmnibusTarge.ps1` is used for building all other Agent packages via Omnibus
- `Build-InstallerPackages.ps1` is used for building the `.exe` installer for the Agent.

These scripts read a few environment variables, notably (non-exhaustive !):

- `OMNIBUS_TARGET` - usually set to `main`
- `TARGET_ARCH` - only `x64` is supported at the moment

/// example
```powershell
docker run -v "$(Get-Location):c:\mnt" -e OMNIBUS_TARGET=main -e TARGET_ARCH=x64 registry.ddbuild.io/ci/datadog-agent-buildimages/windows_ltsc2025_x64:v77240728-510448c3 powershell -C "c:\mnt\tasks\winbuildscripts\Build-AgentPackages.ps1 -BuildOutOfSource 1 -InstallDeps 1 -CheckGoVersion 1"
```
///

If the build succeeds, the build artifacts can be found under `omnibus\pkg` in the repo.
